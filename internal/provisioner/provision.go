package provisioner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mordenhost/whm2bunny/config"
	"github.com/mordenhost/whm2bunny/internal/bunny"
	"github.com/mordenhost/whm2bunny/internal/notifier"
	"github.com/mordenhost/whm2bunny/internal/state"
	"go.uber.org/zap"
)

// Provisioner orchestrates the provisioning of BunnyDNS and BunnyCDN resources
type Provisioner struct {
	bunnyClient  *bunny.Client
	stateManager *state.Manager
	notifier     *notifier.TelegramNotifier
	config       *config.Config
	logger       *zap.Logger

	// Sub-provisioners for specific operations
	domainProvisioner    *DomainProvisioner
	subdomainProvisioner *SubdomainProvisioner
	deprovisioner        *Deprovisioner
}

// NewProvisioner creates a new provisioner with all dependencies
func NewProvisioner(
	cfg *config.Config,
	bunnyClient *bunny.Client,
	stateMgr *state.Manager,
	telegramNotifier *notifier.TelegramNotifier,
	logger *zap.Logger,
) *Provisioner {
	if logger == nil {
		logger = zap.NewNop()
	}

	p := &Provisioner{
		bunnyClient:  bunnyClient,
		stateManager: stateMgr,
		notifier:     telegramNotifier,
		config:       cfg,
		logger:       logger,
	}

	// Initialize sub-provisioners
	p.domainProvisioner = &DomainProvisioner{provisioner: p}
	p.subdomainProvisioner = &SubdomainProvisioner{provisioner: p}
	p.deprovisioner = &Deprovisioner{provisioner: p}

	return p
}

// Provision provisions a new domain with DNS zone, records, and CDN pull zone
// This implements the webhook.Provisioner interface
func (p *Provisioner) Provision(domain, user string) error {
	ctx := context.Background()
	startTime := time.Now()

	p.logger.Info("starting domain provisioning",
		zap.String("domain", domain),
		zap.String("user", user),
	)

	// Check if this domain is already provisioned
	existingState, err := p.stateManager.GetByDomain(domain)
	if err == nil && existingState.Status == state.StatusSuccess {
		p.logger.Info("domain already provisioned, skipping",
			zap.String("domain", domain),
			zap.String("state_id", existingState.ID),
		)
		return nil
	}

	// Create or get existing state for recovery
	var provState *state.ProvisionState
	if existingState != nil {
		// Resume from existing state
		provState = existingState
		p.logger.Info("resuming provisioning from existing state",
			zap.String("domain", domain),
			zap.String("status", provState.Status),
			zap.Int("current_step", provState.CurrentStep),
		)
	} else {
		// Create new provisioning state
		provState = p.stateManager.Create(domain)
	}

	// Mark as provisioning
	if err := p.stateManager.MarkProvisioning(provState.ID); err != nil {
		p.logger.Error("failed to mark state as provisioning",
			zap.String("domain", domain),
			zap.Error(err),
		)
		return fmt.Errorf("failed to mark state as provisioning: %w", err)
	}

	// Execute provisioning steps
	domainProv := &DomainProvisioner{provisioner: p}
	err = domainProv.Provision(ctx, domain, user)

	duration := time.Since(startTime)

	if err != nil {
		// Update state with error
		setErr := p.stateManager.SetError(provState.ID, err.Error())
		if setErr != nil {
			p.logger.Error("failed to set error state",
				zap.String("domain", domain),
				zap.Error(setErr),
			)
		}

		// Send failure notification
		stepName := state.StepName(provState.CurrentStep)
		notifErr := p.notifier.NotifyFailed(ctx, domain, stepName, err.Error())
		if notifErr != nil {
			p.logger.Warn("failed to send failure notification",
				zap.String("domain", domain),
				zap.Error(notifErr),
			)
		}

		return fmt.Errorf("provisioning failed for domain %s: %w", domain, err)
	}

	// Mark as success
	if err := p.stateManager.MarkSuccess(provState.ID); err != nil {
		p.logger.Error("failed to mark state as success",
			zap.String("domain", domain),
			zap.Error(err),
		)
	}

	// Refresh state to get CDN hostname
	finalState, err := p.stateManager.Get(provState.ID)
	if err != nil {
		p.logger.Warn("failed to get final state",
			zap.String("domain", domain),
			zap.Error(err),
		)
	}

	// Send success notification
	cdnHostname := ""
	if finalState != nil {
		cdnHostname = finalState.CDNHostname
	}
	notifErr := p.notifier.NotifySuccess(ctx, domain, finalState.ZoneID, cdnHostname, duration)
	if notifErr != nil {
		p.logger.Warn("failed to send success notification",
			zap.String("domain", domain),
			zap.Error(notifErr),
		)
	}

	p.logger.Info("domain provisioning completed successfully",
		zap.String("domain", domain),
		zap.Duration("duration", duration),
	)

	return nil
}

// ProvisionSubdomain provisions a subdomain under an existing parent domain
// This implements the webhook.Provisioner interface
func (p *Provisioner) ProvisionSubdomain(subdomain, parentDomain, user string) error {
	ctx := context.Background()

	p.logger.Info("starting subdomain provisioning",
		zap.String("subdomain", subdomain),
		zap.String("parent_domain", parentDomain),
		zap.String("user", user),
	)

	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)

	// Check if subdomain is already provisioned
	existingState, err := p.stateManager.GetByDomain(fullDomain)
	if err == nil && existingState.Status == state.StatusSuccess {
		p.logger.Info("subdomain already provisioned, skipping",
			zap.String("subdomain", fullDomain),
		)
		return nil
	}

	// Create new state for subdomain
	var provState *state.ProvisionState
	if existingState != nil {
		provState = existingState
	} else {
		provState = p.stateManager.Create(fullDomain)
	}

	// Mark as provisioning
	if err := p.stateManager.MarkProvisioning(provState.ID); err != nil {
		return fmt.Errorf("failed to mark state as provisioning: %w", err)
	}

	// Execute subdomain provisioning
	subProv := &SubdomainProvisioner{provisioner: p}
	err = subProv.Provision(ctx, subdomain, parentDomain, user)

	if err != nil {
		setErr := p.stateManager.SetError(provState.ID, err.Error())
		if setErr != nil {
			p.logger.Error("failed to set error state",
				zap.String("subdomain", fullDomain),
				zap.Error(setErr),
			)
		}

		notifErr := p.notifier.NotifyFailed(ctx, fullDomain, "subdomain_provisioning", err.Error())
		if notifErr != nil {
			p.logger.Warn("failed to send failure notification",
				zap.String("subdomain", fullDomain),
				zap.Error(notifErr),
			)
		}

		return fmt.Errorf("subdomain provisioning failed: %w", err)
	}

	// Mark as success
	if err := p.stateManager.MarkSuccess(provState.ID); err != nil {
		p.logger.Error("failed to mark state as success",
			zap.String("subdomain", fullDomain),
			zap.Error(err),
		)
	}

	// Refresh state to get CDN hostname
	finalState, err := p.stateManager.Get(provState.ID)
	if err != nil {
		p.logger.Warn("failed to get final state",
			zap.String("subdomain", fullDomain),
			zap.Error(err),
		)
	}

	// Send success notification
	cdnHostname := ""
	if finalState != nil {
		cdnHostname = finalState.CDNHostname
	}
	notifErr := p.notifier.NotifySubdomainProvisioned(ctx, fullDomain, parentDomain, cdnHostname)
	if notifErr != nil {
		p.logger.Warn("failed to send subdomain notification",
			zap.String("subdomain", fullDomain),
			zap.Error(notifErr),
		)
	}

	p.logger.Info("subdomain provisioning completed successfully",
		zap.String("subdomain", fullDomain),
	)

	return nil
}

// Deprovision removes a domain's DNS zone and CDN pull zone
// This implements the webhook.Provisioner interface
func (p *Provisioner) Deprovision(domain string) error {
	ctx := context.Background()

	p.logger.Info("starting domain deprovisioning",
		zap.String("domain", domain),
	)

	// Execute deprovisioning
	deprov := &Deprovisioner{provisioner: p}
	err := deprov.Deprovision(ctx, domain)

	if err != nil {
		p.logger.Error("deprovisioning failed",
			zap.String("domain", domain),
			zap.Error(err),
		)
		return fmt.Errorf("deprovisioning failed for domain %s: %w", domain, err)
	}

	// Remove state
	existingState, stateErr := p.stateManager.GetByDomain(domain)
	if stateErr == nil && existingState != nil {
		if delErr := p.stateManager.Delete(existingState.ID); delErr != nil {
			p.logger.Warn("failed to delete state after deprovisioning",
				zap.String("domain", domain),
				zap.Error(delErr),
			)
		}
	}

	// Send notification
	notifErr := p.notifier.NotifyDeprovisioned(ctx, domain)
	if notifErr != nil {
		p.logger.Warn("failed to send deprovision notification",
			zap.String("domain", domain),
			zap.Error(notifErr),
		)
	}

	p.logger.Info("domain deprovisioning completed successfully",
		zap.String("domain", domain),
	)

	return nil
}

// Recover attempts to recover failed or pending provisions
func (p *Provisioner) Recover(ctx context.Context) error {
	states := p.stateManager.Recover()

	p.logger.Info("starting recovery of pending/failed provisions",
		zap.Int("count", len(states)),
	)

	for _, st := range states {
		p.logger.Info("recovering provision",
			zap.String("domain", st.Domain),
			zap.String("status", st.Status),
			zap.Int("retries", st.Retries),
		)

		// Check if we've exceeded retry limit
		if st.Retries >= 5 {
			p.logger.Warn("skipping recovery, max retries exceeded",
				zap.String("domain", st.Domain),
				zap.Int("retries", st.Retries),
			)
			continue
		}

		// Re-provision the domain
		if err := p.Provision(st.Domain, ""); err != nil {
			p.logger.Error("recovery failed",
				zap.String("domain", st.Domain),
				zap.Error(err),
			)
		}
	}

	return nil
}

// generatePullZoneName generates a pull zone name from a domain
// e.g., "example.com" -> "morden-example-com"
func generatePullZoneName(domain string) string {
	name := strings.ToLower(domain)
	name = strings.ReplaceAll(name, ".", "-")
	return "morden-" + name
}

// generateSubdomainPullZoneName generates a pull zone name for a subdomain
// e.g., "blog.example.com" -> "morden-blog-example-com"
func generateSubdomainPullZoneName(subdomain, parentDomain string) string {
	fullDomain := fmt.Sprintf("%s.%s", subdomain, parentDomain)
	return generatePullZoneName(fullDomain)
}
