package provisioner

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/mordenhost/whm2bunny/internal/bunny"
	"github.com/mordenhost/whm2bunny/internal/state"
)

// Deprovisioner handles deprovisioning (removal) of domains
// This includes:
// 1. Deleting DNS zone
// 2. Deleting CDN pull zone
// 3. Cleaning up state
type Deprovisioner struct {
	provisioner *Provisioner
}

// Deprovision removes all resources associated with a domain
// This process is irreversible - all DNS and CDN resources will be deleted
func (d *Deprovisioner) Deprovision(ctx context.Context, domain string) error {
	d.provisioner.logger.Info("starting deprovisioning",
		zap.String("domain", domain),
	)

	// Get the domain's state to find resource IDs
	provState, err := d.provisioner.stateManager.GetByDomain(domain)
	if err != nil {
		// State not found - domain might not be provisioned
		// Try to find resources by name anyway
		d.provisioner.logger.Warn("state not found for domain, attempting cleanup by name",
			zap.String("domain", domain),
			zap.Error(err),
		)
		return d.deprovisionByName(ctx, domain)
	}

	d.provisioner.logger.Info("found provisioning state",
		zap.String("domain", domain),
		zap.Int64("zone_id", provState.ZoneID),
		zap.Int64("pull_zone_id", provState.PullZoneID),
	)

	// Step 1: Delete DNS zone
	if err := d.deleteDNSZone(ctx, provState.ZoneID, domain); err != nil {
		d.provisioner.logger.Error("failed to delete DNS zone",
			zap.String("domain", domain),
			zap.Error(err),
		)
		// Continue with pull zone deletion even if DNS fails
	}

	// Step 2: Delete pull zone
	if err := d.deletePullZone(ctx, provState.PullZoneID, domain); err != nil {
		d.provisioner.logger.Error("failed to delete pull zone",
			zap.String("domain", domain),
			zap.Error(err),
		)
		// Continue with state cleanup even if pull zone deletion fails
	}

	// Step 3: Clean up state
	if err := d.deleteState(ctx, provState.ID, domain); err != nil {
		d.provisioner.logger.Error("failed to delete state",
			zap.String("domain", domain),
			zap.Error(err),
		)
		return err
	}

	d.provisioner.logger.Info("deprovisioning completed successfully",
		zap.String("domain", domain),
	)

	return nil
}

// deprovisionByName attempts to deprovision a domain by looking up resources by name
// This is used when state is not available
func (d *Deprovisioner) deprovisionByName(ctx context.Context, domain string) error {
	d.provisioner.logger.Info("attempting deprovisioning by name",
		zap.String("domain", domain),
	)

	// Try to find DNS zone by domain
	zone, err := d.provisioner.bunnyClient.GetDNSZone(ctx, domain)
	zoneID := int64(0)
	if err == nil && zone != nil {
		zoneID = zone.ID
		d.provisioner.logger.Info("found DNS zone by name",
			zap.String("domain", domain),
			zap.Int64("zone_id", zoneID),
		)
	}

	// Try to find pull zone by name
	pullZoneName := generatePullZoneName(domain)
	pullZone, err := d.provisioner.bunnyClient.GetPullZoneByName(ctx, pullZoneName)
	pullZoneID := int64(0)
	if err == nil && pullZone != nil {
		pullZoneID = pullZone.ID
		d.provisioner.logger.Info("found pull zone by name",
			zap.String("domain", domain),
			zap.Int64("pull_zone_id", pullZoneID),
		)
	}

	// Delete DNS zone if found
	if zoneID > 0 {
		if err := d.deleteDNSZone(ctx, zoneID, domain); err != nil {
			d.provisioner.logger.Error("failed to delete DNS zone",
				zap.String("domain", domain),
				zap.Error(err),
			)
		}
	}

	// Delete pull zone if found
	if pullZoneID > 0 {
		if err := d.deletePullZone(ctx, pullZoneID, domain); err != nil {
			d.provisioner.logger.Error("failed to delete pull zone",
				zap.String("domain", domain),
				zap.Error(err),
			)
		}
	}

	return nil
}

// deleteDNSZone deletes a DNS zone
func (d *Deprovisioner) deleteDNSZone(ctx context.Context, zoneID int64, domain string) error {
	if zoneID <= 0 {
		d.provisioner.logger.Debug("skipping DNS zone deletion, invalid zone ID",
			zap.String("domain", domain),
		)
		return nil
	}

	d.provisioner.logger.Info("deleting DNS zone",
		zap.String("domain", domain),
		zap.Int64("zone_id", zoneID),
	)

	// First, get all DNS records and log them for audit
	records, err := d.provisioner.bunnyClient.GetDNSRecords(ctx, zoneID)
	if err == nil {
		d.provisioner.logger.Info("DNS zone has records",
			zap.String("domain", domain),
			zap.Int("record_count", len(records)),
		)
	}

	// Delete the zone (this will also delete all records)
	if err := d.provisioner.bunnyClient.DeleteDNSZone(ctx, zoneID); err != nil {
		return fmt.Errorf("failed to delete DNS zone: %w", err)
	}

	d.provisioner.logger.Info("DNS zone deleted successfully",
		zap.String("domain", domain),
		zap.Int64("zone_id", zoneID),
	)

	return nil
}

// deletePullZone deletes a CDN pull zone
func (d *Deprovisioner) deletePullZone(ctx context.Context, pullZoneID int64, domain string) error {
	if pullZoneID <= 0 {
		d.provisioner.logger.Debug("skipping pull zone deletion, invalid zone ID",
			zap.String("domain", domain),
		)
		return nil
	}

	d.provisioner.logger.Info("deleting pull zone",
		zap.String("domain", domain),
		zap.Int64("pull_zone_id", pullZoneID),
	)

	// Get pull zone info before deletion for audit
	pullZone, err := d.provisioner.bunnyClient.GetPullZone(ctx, pullZoneID)
	if err == nil && pullZone != nil {
		d.provisioner.logger.Info("pull zone info",
			zap.String("domain", domain),
			zap.String("zone_name", pullZone.Name),
			zap.Int("hostname_count", len(pullZone.Hostnames)),
		)
	}

	// Delete the pull zone
	if err := d.provisioner.bunnyClient.DeletePullZone(ctx, pullZoneID); err != nil {
		return fmt.Errorf("failed to delete pull zone: %w", err)
	}

	d.provisioner.logger.Info("pull zone deleted successfully",
		zap.String("domain", domain),
		zap.Int64("pull_zone_id", pullZoneID),
	)

	return nil
}

// deleteState removes the provisioning state
func (d *Deprovisioner) deleteState(ctx context.Context, stateID string, domain string) error {
	d.provisioner.logger.Info("deleting provisioning state",
		zap.String("domain", domain),
		zap.String("state_id", stateID),
	)

	if err := d.provisioner.stateManager.Delete(stateID); err != nil {
		return fmt.Errorf("failed to delete state: %w", err)
	}

	d.provisioner.logger.Info("provisioning state deleted successfully",
		zap.String("domain", domain),
		zap.String("state_id", stateID),
	)

	return nil
}

// DeprovisionSubdomain removes a subdomain's resources
// Unlike main domains, subdomains:
// 1. Don't delete the parent DNS zone
// 2. Delete only the subdomain's pull zone
// 3. Remove the CNAME record from the parent zone
func (d *Deprovisioner) DeprovisionSubdomain(ctx context.Context, subdomain, parentDomain string) error {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	d.provisioner.logger.Info("starting subdomain deprovisioning",
		zap.String("subdomain", fullDomain),
	)

	// Get state
	provState, err := d.provisioner.stateManager.GetByDomain(fullDomain)
	if err != nil {
		d.provisioner.logger.Warn("subdomain state not found",
			zap.String("subdomain", fullDomain),
			zap.Error(err),
		)
		// Try to find and delete by name
		return d.deprovisionSubdomainByName(ctx, subdomain, parentDomain)
	}

	// Delete CNAME from parent zone
	if provState.ZoneID > 0 {
		if err := d.deleteSubdomainCNAME(ctx, provState.ZoneID, subdomain, fullDomain); err != nil {
			d.provisioner.logger.Warn("failed to delete subdomain CNAME",
				zap.String("subdomain", fullDomain),
				zap.Error(err),
			)
		}
	}

	// Delete pull zone
	if provState.PullZoneID > 0 {
		if err := d.deletePullZone(ctx, provState.PullZoneID, fullDomain); err != nil {
			d.provisioner.logger.Error("failed to delete subdomain pull zone",
				zap.String("subdomain", fullDomain),
				zap.Error(err),
			)
		}
	}

	// Delete state
	if err := d.provisioner.stateManager.Delete(provState.ID); err != nil {
		d.provisioner.logger.Warn("failed to delete subdomain state",
			zap.String("subdomain", fullDomain),
			zap.Error(err),
		)
	}

	d.provisioner.logger.Info("subdomain deprovisioning completed",
		zap.String("subdomain", fullDomain),
	)

	return nil
}

// deprovisionSubdomainByName attempts to deprovision a subdomain by name lookup
func (d *Deprovisioner) deprovisionSubdomainByName(ctx context.Context, subdomain, parentDomain string) error {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	// Find parent zone
	parentZone, err := d.provisioner.bunnyClient.GetDNSZone(ctx, parentDomain)
	if err == nil && parentZone != nil {
		// Try to delete CNAME from parent zone (ignore errors, best effort)
		_ = d.deleteSubdomainCNAME(ctx, parentZone.ID, subdomain, fullDomain)
	}

	// Find and delete pull zone
	pullZoneName := generateSubdomainPullZoneName(subdomain, parentDomain)
	pullZone, err := d.provisioner.bunnyClient.GetPullZoneByName(ctx, pullZoneName)
	if err == nil && pullZone != nil {
		if err := d.deletePullZone(ctx, pullZone.ID, fullDomain); err != nil {
			return err
		}
	}

	return nil
}

// deleteSubdomainCNAME removes a subdomain's CNAME record from the parent zone
func (d *Deprovisioner) deleteSubdomainCNAME(ctx context.Context, parentZoneID int64, subdomain, fullDomain string) error {
	d.provisioner.logger.Info("deleting subdomain CNAME from parent zone",
		zap.String("subdomain", fullDomain),
		zap.Int64("parent_zone_id", parentZoneID),
	)

	// Get existing records
	records, err := d.provisioner.bunnyClient.GetDNSRecords(ctx, parentZoneID)
	if err != nil {
		return fmt.Errorf("failed to get parent zone records: %w", err)
	}

	// Find and delete the CNAME record
	found := false
	for _, r := range records {
		if r.Name == subdomain && r.Type == bunny.DNSRecordTypeCNAME {
			if err := d.provisioner.bunnyClient.DeleteDNSRecord(ctx, parentZoneID, r.ID); err != nil {
				return fmt.Errorf("failed to delete CNAME record: %w", err)
			}
			found = true
			d.provisioner.logger.Info("deleted subdomain CNAME",
				zap.String("subdomain", fullDomain),
				zap.Int64("record_id", r.ID),
			)
			break
		}
	}

	if !found {
		d.provisioner.logger.Debug("subdomain CNAME not found in parent zone",
			zap.String("subdomain", fullDomain),
		)
	}

	return nil
}

// GetDeprovisionStatus checks if a domain can be safely deprovisioned
func (d *Deprovisioner) GetDeprovisionStatus(ctx context.Context, domain string) (*DeprovisionStatus, error) {
	status := &DeprovisionStatus{
		Domain: domain,
	}

	// Check if domain is provisioned
	provState, err := d.provisioner.stateManager.GetByDomain(domain)
	if err != nil {
		status.Provisioned = false
		status.Message = "Domain is not provisioned"
		return status, nil
	}

	status.Provisioned = true
	status.State = provState

	// Check if DNS zone exists
	if provState.ZoneID > 0 {
		zone, err := d.provisioner.bunnyClient.GetDNSZoneByID(ctx, provState.ZoneID)
		if err == nil && zone != nil {
			status.DNSZoneExists = true
		}
	}

	// Check if pull zone exists
	if provState.PullZoneID > 0 {
		pullZone, err := d.provisioner.bunnyClient.GetPullZone(ctx, provState.PullZoneID)
		if err == nil && pullZone != nil {
			status.PullZoneExists = true
		}
	}

	status.CanDeprovision = true
	return status, nil
}

// DeprovisionStatus represents the deprovisioning status of a domain
type DeprovisionStatus struct {
	Domain         string
	Provisioned    bool
	DNSZoneExists  bool
	PullZoneExists bool
	CanDeprovision bool
	Message        string
	State          *state.ProvisionState
}
