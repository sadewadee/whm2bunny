package provisioner

import (
	"context"
	"fmt"
	"strings"

	"github.com/mordenhost/whm2bunny/internal/bunny"
	"github.com/mordenhost/whm2bunny/internal/state"
	"go.uber.org/zap"
)

const (
	// Subdomain pull zone uses special configuration
	subdomainCacheExpiration = 1440 // 24 hours
)

// SubdomainProvisioner handles provisioning for subdomains
// Unlike main domains, subdomains:
// 1. Don't create a new DNS zone (use parent's zone)
// 2. Create a pull zone for the subdomain
// 3. Add CNAME record in parent zone pointing to CDN
type SubdomainProvisioner struct {
	provisioner *Provisioner
}

// Provision provisions a subdomain with CDN pull zone
// This is a 2-step process:
// Step 1: Find parent DNS zone and create pull zone for subdomain
// Step 2: Add CNAME record in parent zone
func (s *SubdomainProvisioner) Provision(ctx context.Context, subdomain, parentDomain, user string) error {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	// Get or create state for this subdomain
	provState, err := s.provisioner.stateManager.GetByDomain(fullDomain)
	if err != nil {
		return fmt.Errorf("failed to get state: %w", err)
	}

	// Resume from the last successful step
	switch provState.CurrentStep {
	case state.StepNone, state.StepDNSZone:
		// For subdomains, StepDNSZone means finding parent and creating pull zone
		if err := s.findParentAndCreatePullZone(ctx, subdomain, parentDomain, provState); err != nil {
			return fmt.Errorf("failed to find parent zone and create pull zone: %w", err)
		}
		fallthrough

	case state.StepDNSRecords, state.StepPullZone:
		// For subdomains, this step means adding the CNAME to parent zone
		if err := s.addSubdomainCNAME(ctx, subdomain, parentDomain, provState); err != nil {
			return fmt.Errorf("failed to add subdomain CNAME: %w", err)
		}

	default:
		// Already completed
		s.provisioner.logger.Info("subdomain provisioning already completed",
			zap.String("subdomain", fullDomain),
		)
	}

	return nil
}

// findParentAndCreatePullZone finds the parent DNS zone and creates a pull zone for the subdomain
// Step 1 of subdomain provisioning
func (s *SubdomainProvisioner) findParentAndCreatePullZone(ctx context.Context, subdomain, parentDomain string, provState *state.ProvisionState) error {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	s.provisioner.logger.Info("finding parent DNS zone",
		zap.String("subdomain", fullDomain),
		zap.String("parent_domain", parentDomain),
	)

	// Find parent DNS zone
	parentZone, err := s.provisioner.bunnyClient.GetDNSZone(ctx, parentDomain)
	if err != nil {
		s.provisioner.logger.Error("parent DNS zone not found",
			zap.String("parent_domain", parentDomain),
			zap.Error(err),
		)
		return fmt.Errorf("parent DNS zone not found for %s: %w", parentDomain, err)
	}

	s.provisioner.logger.Info("found parent DNS zone",
		zap.String("parent_domain", parentDomain),
		zap.Int64("zone_id", parentZone.ID),
	)

	// Store parent zone ID in state (we use ZoneID field for parent)
	provState.ZoneID = parentZone.ID
	if err := s.provisioner.stateManager.Update(provState); err != nil {
		return err
	}

	// Create pull zone for subdomain
	pullZoneName := generateSubdomainPullZoneName(subdomain, parentDomain)

	// Check if pull zone already exists
	existingZone, err := s.provisioner.bunnyClient.GetPullZoneByName(ctx, pullZoneName)
	if err == nil && existingZone != nil {
		s.provisioner.logger.Info("subdomain pull zone already exists, reusing",
			zap.String("subdomain", fullDomain),
			zap.String("zone_name", pullZoneName),
			zap.Int64("zone_id", existingZone.ID),
		)
		provState.PullZoneID = existingZone.ID
		provState.CDNHostname = s.extractCDNHostname(existingZone)
		if updateErr := s.provisioner.stateManager.Update(provState); updateErr != nil {
			return updateErr
		}
		// Skip to CNAME step
		if err := s.provisioner.stateManager.Update(provState); err != nil {
			return err
		}
		provState.CurrentStep = state.StepPullZone
		return nil
	}

	// Create the pull zone
	originIP := s.provisioner.config.Origin.ReverseProxyIP
	pullZone, err := s.provisioner.bunnyClient.CreatePullZone(ctx, fullDomain, originIP)
	if err != nil {
		s.provisioner.logger.Error("failed to create pull zone for subdomain",
			zap.String("subdomain", fullDomain),
			zap.Error(err),
		)
		return err
	}

	// Add subdomain hostname to pull zone
	if err := s.provisioner.bunnyClient.AddPullZoneHostname(ctx, pullZone.ID, fullDomain); err != nil {
		s.provisioner.logger.Warn("failed to add hostname to subdomain pull zone",
			zap.String("subdomain", fullDomain),
			zap.Int64("pull_zone_id", pullZone.ID),
			zap.Error(err),
		)
		// Don't fail on hostname error
	}

	// Extract CDN hostname
	cdnHostname := s.extractCDNHostname(pullZone)

	// Update state
	provState.PullZoneID = pullZone.ID
	provState.CDNHostname = cdnHostname
	provState.CurrentStep = state.StepPullZone
	if err := s.provisioner.stateManager.Update(provState); err != nil {
		return err
	}

	s.provisioner.logger.Info("subdomain pull zone created successfully",
		zap.String("subdomain", fullDomain),
		zap.String("zone_name", pullZoneName),
		zap.Int64("zone_id", pullZone.ID),
		zap.String("cdn_hostname", cdnHostname),
	)

	return nil
}

// addSubdomainCNAME adds a CNAME record in the parent zone pointing to the CDN
// Step 2 of subdomain provisioning
func (s *SubdomainProvisioner) addSubdomainCNAME(ctx context.Context, subdomain, parentDomain string, provState *state.ProvisionState) error {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	s.provisioner.logger.Info("adding subdomain CNAME to parent zone",
		zap.String("subdomain", fullDomain),
		zap.Int64("parent_zone_id", provState.ZoneID),
	)

	// Get pull zone to verify CDN hostname
	pullZone, err := s.provisioner.bunnyClient.GetPullZone(ctx, provState.PullZoneID)
	if err != nil {
		return fmt.Errorf("failed to get pull zone: %w", err)
	}

	cdnHostname := s.extractCDNHostname(pullZone)
	if cdnHostname == "" {
		return fmt.Errorf("could not extract CDN hostname from pull zone")
	}

	// Get existing records in parent zone
	existingRecords, err := s.provisioner.bunnyClient.GetDNSRecords(ctx, provState.ZoneID)
	if err != nil {
		s.provisioner.logger.Warn("failed to get existing DNS records",
			zap.Int64("zone_id", provState.ZoneID),
			zap.Error(err),
		)
		existingRecords = nil
	}

	// Check if subdomain CNAME already exists
	cnameExists := false
	for _, r := range existingRecords {
		if r.Name == subdomain && r.Type == bunny.DNSRecordTypeCNAME {
			cnameExists = true
			s.provisioner.logger.Info("subdomain CNAME already exists, updating",
				zap.String("subdomain", subdomain),
				zap.String("old_value", r.Value),
				zap.String("new_value", cdnHostname),
			)
			// Update existing record
			updateReq := &bunny.UpdateDNSRecordRequest{
				Type:    bunny.DNSRecordTypeCNAME,
				Name:    subdomain,
				Value:   cdnHostname,
				TTL:     defaultDNSRecordTTL,
				Enabled: true,
			}
			if err := s.provisioner.bunnyClient.UpdateDNSRecord(ctx, provState.ZoneID, r.ID, updateReq); err != nil {
				return fmt.Errorf("failed to update subdomain CNAME: %w", err)
			}
			break
		}
	}

	// Add new CNAME if it doesn't exist
	if !cnameExists {
		cnameRecord := &bunny.AddDNSRecordRequest{
			Type:    bunny.DNSRecordTypeCNAME,
			Name:    subdomain,
			Value:   cdnHostname,
			TTL:     defaultDNSRecordTTL,
			Enabled: true,
		}
		if _, err := s.provisioner.bunnyClient.AddDNSRecord(ctx, provState.ZoneID, cnameRecord); err != nil {
			return fmt.Errorf("failed to add subdomain CNAME: %w", err)
		}
	}

	// Mark as completed
	provState.CurrentStep = state.StepCNAMESync
	if err := s.provisioner.stateManager.Update(provState); err != nil {
		return err
	}

	s.provisioner.logger.Info("subdomain CNAME added successfully",
		zap.String("subdomain", fullDomain),
		zap.String("cdn_hostname", cdnHostname),
	)

	return nil
}

// extractCDNHostname extracts the CDN hostname from a pull zone
func (s *SubdomainProvisioner) extractCDNHostname(pullZone *bunny.PullZone) string {
	if pullZone == nil {
		return ""
	}

	// Check if we have hostnames in the pull zone
	if len(pullZone.Hostnames) > 0 {
		// The first hostname is usually the CDN hostname
		for _, h := range pullZone.Hostnames {
			if strings.Contains(h.Hostname, ".bunnycdn.com") {
				return h.Hostname
			}
		}
		// If no bunnycdn.com hostname, return the first one
		if pullZone.Hostnames[0].Hostname != "" {
			return pullZone.Hostnames[0].Hostname
		}
	}

	// Fallback: construct hostname from zone ID
	if pullZone.ID > 0 {
		return fmt.Sprintf("%d.bunnycdn.com", pullZone.ID)
	}

	return ""
}

// GetSubdomainStatus returns the current provisioning status for a subdomain
func (s *SubdomainProvisioner) GetSubdomainStatus(ctx context.Context, subdomain, parentDomain string) (*state.ProvisionState, error) {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)
	return s.provisioner.stateManager.GetByDomain(fullDomain)
}

// RemoveSubdomain removes a subdomain's pull zone and CNAME record
func (s *SubdomainProvisioner) RemoveSubdomain(ctx context.Context, subdomain, parentDomain string) error {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	s.provisioner.logger.Info("removing subdomain",
		zap.String("subdomain", fullDomain),
	)

	// Get state to find resources
	provState, err := s.provisioner.stateManager.GetByDomain(fullDomain)
	if err != nil {
		return fmt.Errorf("subdomain state not found: %w", err)
	}

	// Find and delete CNAME record from parent zone
	if provState.ZoneID > 0 {
		existingRecords, err := s.provisioner.bunnyClient.GetDNSRecords(ctx, provState.ZoneID)
		if err == nil {
			for _, r := range existingRecords {
				if r.Name == subdomain && r.Type == bunny.DNSRecordTypeCNAME {
					if err := s.provisioner.bunnyClient.DeleteDNSRecord(ctx, provState.ZoneID, r.ID); err != nil {
						s.provisioner.logger.Warn("failed to delete subdomain CNAME",
							zap.String("subdomain", subdomain),
							zap.Error(err),
						)
					} else {
						s.provisioner.logger.Info("deleted subdomain CNAME",
							zap.String("subdomain", subdomain),
						)
					}
					break
				}
			}
		}
	}

	// Delete pull zone
	if provState.PullZoneID > 0 {
		if err := s.provisioner.bunnyClient.DeletePullZone(ctx, provState.PullZoneID); err != nil {
			s.provisioner.logger.Warn("failed to delete subdomain pull zone",
				zap.String("subdomain", fullDomain),
				zap.Int64("pull_zone_id", provState.PullZoneID),
				zap.Error(err),
			)
			// Continue anyway
		} else {
			s.provisioner.logger.Info("deleted subdomain pull zone",
				zap.String("subdomain", fullDomain),
				zap.Int64("pull_zone_id", provState.PullZoneID),
			)
		}
	}

	// Remove state
	if err := s.provisioner.stateManager.Delete(provState.ID); err != nil {
		s.provisioner.logger.Warn("failed to delete subdomain state",
			zap.String("subdomain", fullDomain),
			zap.Error(err),
		)
	}

	return nil
}
