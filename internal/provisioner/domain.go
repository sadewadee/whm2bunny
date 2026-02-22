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
	// Default TTL for DNS records (1 hour)
	defaultDNSRecordTTL = 3600
)

// DomainProvisioner handles provisioning for main domains and addon domains
type DomainProvisioner struct {
	provisioner *Provisioner
}

// Provision provisions a domain with DNS zone, records, and CDN pull zone
// This is a 4-step process:
// Step 1: Create DNS Zone
// Step 2: Add DNS Records (A, CNAME, MX, TXT)
// Step 3: Create Pull Zone
// Step 4: Sync CDN CNAME to DNS
func (d *DomainProvisioner) Provision(ctx context.Context, domain, user string) error {
	// Get or create state for this domain
	provState, err := d.provisioner.stateManager.GetByDomain(domain)
	if err != nil {
		return fmt.Errorf("failed to get state: %w", err)
	}

	// Resume from the last successful step
	switch provState.CurrentStep {
	case state.StepNone, state.StepDNSZone:
		if err := d.createDNSZone(ctx, domain, provState); err != nil {
			return fmt.Errorf("failed to create DNS zone: %w", err)
		}
		fallthrough

	case state.StepDNSRecords:
		if err := d.addDNSRecords(ctx, provState.ZoneID, domain, provState); err != nil {
			return fmt.Errorf("failed to add DNS records: %w", err)
		}
		fallthrough

	case state.StepPullZone:
		if err := d.createPullZone(ctx, domain, provState); err != nil {
			return fmt.Errorf("failed to create pull zone: %w", err)
		}
		fallthrough

	case state.StepCNAMESync:
		if err := d.syncCDNCNAME(ctx, provState.ZoneID, provState.PullZoneID, provState); err != nil {
			return fmt.Errorf("failed to sync CDN CNAME: %w", err)
		}

	default:
		// Already completed
		d.provisioner.logger.Info("domain provisioning already completed",
			zap.String("domain", domain),
		)
	}

	return nil
}

// createDNSZone creates a BunnyDNS zone for the domain
// Step 1 of the provisioning process
func (d *DomainProvisioner) createDNSZone(ctx context.Context, domain string, provState *state.ProvisionState) error {
	d.provisioner.logger.Info("creating DNS zone",
		zap.String("domain", domain),
		zap.String("state_id", provState.ID),
	)

	// Check if zone already exists (idempotency)
	existingZone, err := d.provisioner.bunnyClient.GetDNSZone(ctx, domain)
	if err == nil && existingZone != nil {
		d.provisioner.logger.Info("DNS zone already exists, reusing",
			zap.String("domain", domain),
			zap.Int64("zone_id", existingZone.ID),
		)
		provState.ZoneID = existingZone.ID
		if updateErr := d.provisioner.stateManager.Update(provState); updateErr != nil {
			return updateErr
		}
		if err := d.provisioner.stateManager.IncrementStep(provState.ID); err != nil {
			return err
		}
		return nil
	}

	// Create the DNS zone
	soaEmail := d.provisioner.config.DNS.SOAEmail
	zone, err := d.provisioner.bunnyClient.CreateDNSZone(ctx, domain, soaEmail)
	if err != nil {
		d.provisioner.logger.Error("failed to create DNS zone",
			zap.String("domain", domain),
			zap.Error(err),
		)
		return err
	}

	// Update state with zone ID
	provState.ZoneID = zone.ID
	if err := d.provisioner.stateManager.Update(provState); err != nil {
		return err
	}

	// Advance to next step
	if err := d.provisioner.stateManager.IncrementStep(provState.ID); err != nil {
		return err
	}

	d.provisioner.logger.Info("DNS zone created successfully",
		zap.String("domain", domain),
		zap.Int64("zone_id", zone.ID),
	)

	return nil
}

// addDNSRecords adds standard DNS records to the zone
// Step 2 of the provisioning process
// Adds:
// - A record: @ -> reverseProxyIP
// - CNAME: www -> @
// - MX record: 10 mail.domain.com
// - TXT: v=spf1 a mx -all
func (d *DomainProvisioner) addDNSRecords(ctx context.Context, zoneID int64, domain string, provState *state.ProvisionState) error {
	d.provisioner.logger.Info("adding DNS records",
		zap.String("domain", domain),
		zap.Int64("zone_id", zoneID),
	)

	reverseProxyIP := d.provisioner.config.Origin.ReverseProxyIP

	// Get existing records to check for duplicates
	existingRecords, err := d.provisioner.bunnyClient.GetDNSRecords(ctx, zoneID)
	if err != nil {
		d.provisioner.logger.Warn("failed to get existing DNS records, continuing anyway",
			zap.Int64("zone_id", zoneID),
			zap.Error(err),
		)
		existingRecords = nil
	}

	recordExists := func(name string, recordType bunny.DNSRecordType) bool {
		for _, r := range existingRecords {
			if r.Name == name && r.Type == recordType {
				return true
			}
		}
		return false
	}

	// Add A record: @ -> reverseProxyIP
	if !recordExists("@", bunny.DNSRecordTypeA) {
		aRecord := &bunny.AddDNSRecordRequest{
			Type:    bunny.DNSRecordTypeA,
			Name:    "@",
			Value:   reverseProxyIP,
			TTL:     defaultDNSRecordTTL,
			Enabled: true,
		}
		if _, err := d.provisioner.bunnyClient.AddDNSRecord(ctx, zoneID, aRecord); err != nil {
			return fmt.Errorf("failed to add A record: %w", err)
		}
		d.provisioner.logger.Debug("added A record",
			zap.String("domain", domain),
			zap.String("name", "@"),
			zap.String("value", reverseProxyIP),
		)
	} else {
		d.provisioner.logger.Debug("A record already exists, skipping",
			zap.String("domain", domain),
		)
	}

	// Add CNAME: www -> @
	if !recordExists("www", bunny.DNSRecordTypeCNAME) {
		cnameRecord := &bunny.AddDNSRecordRequest{
			Type:    bunny.DNSRecordTypeCNAME,
			Name:    "www",
			Value:   domain + ".",
			TTL:     defaultDNSRecordTTL,
			Enabled: true,
		}
		if _, err := d.provisioner.bunnyClient.AddDNSRecord(ctx, zoneID, cnameRecord); err != nil {
			return fmt.Errorf("failed to add www CNAME record: %w", err)
		}
		d.provisioner.logger.Debug("added www CNAME record",
			zap.String("domain", domain),
		)
	} else {
		d.provisioner.logger.Debug("www CNAME record already exists, skipping",
			zap.String("domain", domain),
		)
	}

	// Add MX record: 10 mail.domain.com
	if !recordExists("@", bunny.DNSRecordTypeMX) {
		mxRecord := &bunny.AddDNSRecordRequest{
			Type:     bunny.DNSRecordTypeMX,
			Name:     "@",
			Value:    fmt.Sprintf("mail.%s.", domain),
			Priority: 10,
			TTL:      defaultDNSRecordTTL,
			Enabled:  true,
		}
		if _, err := d.provisioner.bunnyClient.AddDNSRecord(ctx, zoneID, mxRecord); err != nil {
			return fmt.Errorf("failed to add MX record: %w", err)
		}
		d.provisioner.logger.Debug("added MX record",
			zap.String("domain", domain),
		)
	} else {
		d.provisioner.logger.Debug("MX record already exists, skipping",
			zap.String("domain", domain),
		)
	}

	// Add TXT record: v=spf1 a mx -all
	if !recordExists("@", bunny.DNSRecordTypeTXT) {
		txtRecord := &bunny.AddDNSRecordRequest{
			Type:    bunny.DNSRecordTypeTXT,
			Name:    "@",
			Value:   "v=spf1 a mx -all",
			TTL:     defaultDNSRecordTTL,
			Enabled: true,
		}
		if _, err := d.provisioner.bunnyClient.AddDNSRecord(ctx, zoneID, txtRecord); err != nil {
			return fmt.Errorf("failed to add SPF TXT record: %w", err)
		}
		d.provisioner.logger.Debug("added SPF TXT record",
			zap.String("domain", domain),
		)
	} else {
		d.provisioner.logger.Debug("SPF TXT record already exists, skipping",
			zap.String("domain", domain),
		)
	}

	// Add DMARC TXT record if not exists
	dmarcName := "_dmarc"
	if !recordExists(dmarcName, bunny.DNSRecordTypeTXT) {
		dmarcRecord := &bunny.AddDNSRecordRequest{
			Type:    bunny.DNSRecordTypeTXT,
			Name:    dmarcName,
			Value:   "v=DMARC1; p=none; rua=mailto:dmarc@" + domain,
			TTL:     defaultDNSRecordTTL,
			Enabled: true,
		}
		if _, err := d.provisioner.bunnyClient.AddDNSRecord(ctx, zoneID, dmarcRecord); err != nil {
			// Don't fail on DMARC error, just log
			d.provisioner.logger.Warn("failed to add DMARC record",
				zap.String("domain", domain),
				zap.Error(err),
			)
		} else {
			d.provisioner.logger.Debug("added DMARC TXT record",
				zap.String("domain", domain),
			)
		}
	}

	// Advance to next step
	if err := d.provisioner.stateManager.IncrementStep(provState.ID); err != nil {
		return err
	}

	d.provisioner.logger.Info("DNS records added successfully",
		zap.String("domain", domain),
		zap.Int64("zone_id", zoneID),
	)

	return nil
}

// createPullZone creates a BunnyCDN pull zone for the domain
// Step 3 of the provisioning process
func (d *DomainProvisioner) createPullZone(ctx context.Context, domain string, provState *state.ProvisionState) error {
	d.provisioner.logger.Info("creating pull zone",
		zap.String("domain", domain),
	)

	// Generate pull zone name
	zoneName := generatePullZoneName(domain)

	// Check if pull zone already exists (idempotency)
	existingZone, err := d.provisioner.bunnyClient.GetPullZoneByName(ctx, zoneName)
	if err == nil && existingZone != nil {
		d.provisioner.logger.Info("pull zone already exists, reusing",
			zap.String("domain", domain),
			zap.String("zone_name", zoneName),
			zap.Int64("zone_id", existingZone.ID),
		)
		provState.PullZoneID = existingZone.ID
		provState.CDNHostname = d.extractCDNHostname(existingZone)
		if updateErr := d.provisioner.stateManager.Update(provState); updateErr != nil {
			return updateErr
		}
		if err := d.provisioner.stateManager.IncrementStep(provState.ID); err != nil {
			return err
		}
		return nil
	}

	// Create the pull zone
	originIP := d.provisioner.config.Origin.ReverseProxyIP
	pullZone, err := d.provisioner.bunnyClient.CreatePullZone(ctx, domain, originIP)
	if err != nil {
		d.provisioner.logger.Error("failed to create pull zone",
			zap.String("domain", domain),
			zap.Error(err),
		)
		return err
	}

	// Add domain hostname to pull zone
	if err := d.provisioner.bunnyClient.AddPullZoneHostname(ctx, pullZone.ID, domain); err != nil {
		d.provisioner.logger.Warn("failed to add hostname to pull zone",
			zap.String("domain", domain),
			zap.Int64("pull_zone_id", pullZone.ID),
			zap.Error(err),
		)
		// Don't fail on hostname error, the zone is still usable
	}

	// Extract CDN hostname from pull zone
	cdnHostname := d.extractCDNHostname(pullZone)

	// Update state
	provState.PullZoneID = pullZone.ID
	provState.CDNHostname = cdnHostname
	if err := d.provisioner.stateManager.Update(provState); err != nil {
		return err
	}

	// Advance to next step
	if err := d.provisioner.stateManager.IncrementStep(provState.ID); err != nil {
		return err
	}

	d.provisioner.logger.Info("pull zone created successfully",
		zap.String("domain", domain),
		zap.String("zone_name", zoneName),
		zap.Int64("zone_id", pullZone.ID),
		zap.String("cdn_hostname", cdnHostname),
	)

	return nil
}

// syncCDNCNAME adds a CNAME record pointing 'cdn' to the CDN hostname
// Step 4 of the provisioning process
func (d *DomainProvisioner) syncCDNCNAME(ctx context.Context, zoneID int64, pullZoneID int64, provState *state.ProvisionState) error {
	d.provisioner.logger.Info("syncing CDN CNAME",
		zap.Int64("zone_id", zoneID),
		zap.Int64("pull_zone_id", pullZoneID),
	)

	// Get pull zone to get hostname
	pullZone, err := d.provisioner.bunnyClient.GetPullZone(ctx, pullZoneID)
	if err != nil {
		return fmt.Errorf("failed to get pull zone: %w", err)
	}

	cdnHostname := d.extractCDNHostname(pullZone)
	if cdnHostname == "" {
		return fmt.Errorf("could not extract CDN hostname from pull zone")
	}

	// Get existing records to check for duplicates
	existingRecords, err := d.provisioner.bunnyClient.GetDNSRecords(ctx, zoneID)
	if err != nil {
		d.provisioner.logger.Warn("failed to get existing DNS records",
			zap.Int64("zone_id", zoneID),
			zap.Error(err),
		)
		existingRecords = nil
	}

	// Check if 'cdn' CNAME already exists
	cnameExists := false
	for _, r := range existingRecords {
		if r.Name == "cdn" && r.Type == bunny.DNSRecordTypeCNAME {
			cnameExists = true
			d.provisioner.logger.Info("cdn CNAME already exists, updating value",
				zap.Int64("zone_id", zoneID),
				zap.String("old_value", r.Value),
				zap.String("new_value", cdnHostname),
			)
			// Update existing record
			updateReq := &bunny.UpdateDNSRecordRequest{
				Type:    bunny.DNSRecordTypeCNAME,
				Name:    "cdn",
				Value:   cdnHostname,
				TTL:     defaultDNSRecordTTL,
				Enabled: true,
			}
			if err := d.provisioner.bunnyClient.UpdateDNSRecord(ctx, zoneID, r.ID, updateReq); err != nil {
				return fmt.Errorf("failed to update cdn CNAME record: %w", err)
			}
			break
		}
	}

	// Add new CNAME if it doesn't exist
	if !cnameExists {
		cnameRecord := &bunny.AddDNSRecordRequest{
			Type:    bunny.DNSRecordTypeCNAME,
			Name:    "cdn",
			Value:   cdnHostname,
			TTL:     defaultDNSRecordTTL,
			Enabled: true,
		}
		if _, err := d.provisioner.bunnyClient.AddDNSRecord(ctx, zoneID, cnameRecord); err != nil {
			return fmt.Errorf("failed to add cdn CNAME record: %w", err)
		}
	}

	// Update state with CDN hostname
	provState.CDNHostname = cdnHostname
	if err := d.provisioner.stateManager.Update(provState); err != nil {
		return err
	}

	// Advance to next step
	if err := d.provisioner.stateManager.IncrementStep(provState.ID); err != nil {
		return err
	}

	d.provisioner.logger.Info("CDN CNAME synced successfully",
		zap.Int64("zone_id", zoneID),
		zap.String("cdn_hostname", cdnHostname),
	)

	return nil
}

// extractCDNHostname extracts the CDN hostname from a pull zone
// BunnyCDN typically returns hostnames like: xxxxx.bunnycdn.com
func (d *DomainProvisioner) extractCDNHostname(pullZone *bunny.PullZone) string {
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

	// Fallback: construct hostname from zone name
	// BunnyCDN typically uses: {zone-id}.bunnycdn.com
	if pullZone.ID > 0 {
		return fmt.Sprintf("%d.bunnycdn.com", pullZone.ID)
	}

	return ""
}

// GetProvisionStatus returns the current provisioning status for a domain
func (d *DomainProvisioner) GetProvisionStatus(ctx context.Context, domain string) (*state.ProvisionState, error) {
	return d.provisioner.stateManager.GetByDomain(domain)
}
