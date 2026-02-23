package validator

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mordenhost/whm2bunny/internal/webhook"
)

const (
	// maxDomainLength is the maximum length of a domain name
	maxDomainLength = 253
	// maxLabelLength is the maximum length of a single label
	maxLabelLength = 63
	// maxSubdomainLabels is the maximum number of labels in a subdomain
	maxSubdomainLabels = 5
)

// Regular expressions for domain validation
var (
	// domainLabelRegex matches a valid domain label (RFC 1035)
	// Allows letters, digits, and hyphens (but not at start/end)
	domainLabelRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

	// domainRegex matches a valid domain name
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

	// ipRegex matches a valid IPv4 address
	ipRegex = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)

	// hostnameRegex matches a valid hostname (allows trailing dot)
	hostnameRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)*[a-zA-Z]{2,}\.?$`)
)

// ValidationResult contains the result of validation
type ValidationResult struct {
	Valid   bool
	Message string
}

// Validator validates input data with DNS checks
type Validator struct {
	enableDNSChecks bool
	dnsTimeout      time.Duration
	logger          *zap.Logger
}

// NewValidator creates a validator with default settings
// Deprecated: Use NewValidator with config for more control
func NewValidator() *Validator {
	return NewValidatorWithConfig(nil, nil)
}

// NewValidatorWithConfig creates a validator with configuration
func NewValidatorWithConfig(cfg *ValidatorConfig, logger *zap.Logger) *Validator {
	if cfg == nil {
		cfg = DefaultValidatorConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Validator{
		enableDNSChecks: cfg.EnableDNSChecks,
		dnsTimeout:      cfg.DNSTimeout,
		logger:          logger,
	}
}

// ValidatorConfig contains configuration for the validator
type ValidatorConfig struct {
	EnableDNSChecks bool
	DNSTimeout      time.Duration
}

// DefaultValidatorConfig returns default validator configuration
func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		EnableDNSChecks: true,
		DNSTimeout:      5 * time.Second,
	}
}

// ValidateDomain validates a domain name format and DNS records
func (v *Validator) ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is required")
	}

	// Trim whitespace and trailing dot
	domain = strings.TrimSpace(domain)
	domain = strings.TrimSuffix(domain, ".")

	// Check length
	if len(domain) > maxDomainLength {
		return fmt.Errorf("domain too long (max %d characters)", maxDomainLength)
	}

	// Check format
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain format")
	}

	// Check each label
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if len(label) > maxLabelLength {
			return fmt.Errorf("label '%s' too long (max %d characters)", label, maxLabelLength)
		}
		if !domainLabelRegex.MatchString(label) {
			return fmt.Errorf("label '%s' contains invalid characters", label)
		}
	}

	// Check TLD is not all numeric
	tld := labels[len(labels)-1]
	if v.isAllNumeric(tld) {
		return fmt.Errorf("TLD cannot be all numeric")
	}

	// DNS checks if enabled
	if v.enableDNSChecks {
		if err := v.validateDomainDNS(domain); err != nil {
			v.logger.Warn("domain DNS validation failed",
				zap.String("domain", domain),
				zap.Error(err),
			)
			// Don't fail on DNS errors, just warn
		}
	}

	return nil
}

// ValidateSubdomain validates a subdomain name
func (v *Validator) ValidateSubdomain(subdomain string) error {
	if subdomain == "" {
		return fmt.Errorf("subdomain is required")
	}

	// Trim whitespace
	subdomain = strings.TrimSpace(subdomain)
	subdomain = strings.TrimSuffix(subdomain, ".")

	// Check length
	if len(subdomain) > maxDomainLength {
		return fmt.Errorf("subdomain too long (max %d characters)", maxDomainLength)
	}

	// Must have at least 2 labels (subdomain.parent)
	labels := strings.Split(subdomain, ".")
	if len(labels) < 2 {
		return fmt.Errorf("subdomain must have at least one dot")
	}

	if len(labels) > maxSubdomainLabels+2 { // +2 for parent domain
		return fmt.Errorf("too many labels in subdomain (max %d)", maxSubdomainLabels)
	}

	// Validate each label
	for _, label := range labels {
		if len(label) > maxLabelLength {
			return fmt.Errorf("label '%s' too long (max %d characters)", label, maxLabelLength)
		}
		if !domainLabelRegex.MatchString(label) {
			return fmt.Errorf("label '%s' contains invalid characters", label)
		}
	}

	// Check parent domain TLD
	tld := labels[len(labels)-1]
	if v.isAllNumeric(tld) {
		return fmt.Errorf("TLD cannot be all numeric")
	}

	return nil
}

// ValidateWebhookPayload validates webhook payload structure and required fields
func (v *Validator) ValidateWebhookPayload(payload *webhook.WebhookPayload) error {
	if payload == nil {
		return fmt.Errorf("payload is required")
	}

	// Validate event type
	validEvents := map[string]bool{
		"account_created":   true,
		"addon_created":     true,
		"subdomain_created": true,
		"account_deleted":   true,
	}

	if !validEvents[payload.Event] {
		return fmt.Errorf("invalid event type: '%s'", payload.Event)
	}

	// Validate user (always required)
	if payload.User == "" {
		return fmt.Errorf("user is required")
	}

	// Event-specific validation
	switch payload.Event {
	case "account_created", "addon_created", "account_deleted":
		if payload.Domain == "" {
			return fmt.Errorf("domain is required for event '%s'", payload.Event)
		}
		// Validate domain format
		if err := v.ValidateDomain(payload.Domain); err != nil {
			return fmt.Errorf("invalid domain: %w", err)
		}

	case "subdomain_created":
		if payload.Subdomain == "" {
			return fmt.Errorf("subdomain is required for event '%s'", payload.Event)
		}
		if payload.ParentDomain == "" {
			return fmt.Errorf("parent_domain is required for event '%s'", payload.Event)
		}
		// Validate parent domain
		if err := v.ValidateDomain(payload.ParentDomain); err != nil {
			return fmt.Errorf("invalid parent domain: %w", err)
		}
		// Validate subdomain label only
		if err := v.validateSubdomainLabel(payload.Subdomain); err != nil {
			return fmt.Errorf("invalid subdomain: %w", err)
		}
	}

	return nil
}

// validateDomainDNS performs DNS validation for a domain
func (v *Validator) validateDomainDNS(domain string) error {
	ctx, cancel := context.WithTimeout(context.Background(), v.dnsTimeout)
	defer cancel()

	// Check if domain has any DNS records
	_, err := net.DefaultResolver.LookupHost(ctx, domain)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}

	return nil
}

// validateSubdomainLabel validates just the subdomain label (not full domain)
func (v *Validator) validateSubdomainLabel(label string) error {
	if label == "" {
		return fmt.Errorf("subdomain label is required")
	}

	if len(label) > maxLabelLength {
		return fmt.Errorf("subdomain label too long (max %d characters)", maxLabelLength)
	}

	if !domainLabelRegex.MatchString(label) {
		return fmt.Errorf("subdomain label contains invalid characters")
	}

	return nil
}

// isAllNumeric checks if a string contains only digits
func (v *Validator) isAllNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ValidateOriginIP validates an origin IP address
func (v *Validator) ValidateOriginIP(ip string) error {
	if ip == "" {
		return fmt.Errorf("origin IP is required")
	}

	// Check IPv4 format
	if ipRegex.MatchString(ip) {
		parts := strings.Split(ip, ".")
		for _, part := range parts {
			var num int
			if _, err := fmt.Sscanf(part, "%d", &num); err != nil {
				return fmt.Errorf("invalid IP address octet: %s", part)
			}
			if num < 0 || num > 255 {
				return fmt.Errorf("invalid IP address octet: %s", part)
			}
		}
		return nil
	}

	// Check if it's a valid hostname
	if hostnameRegex.MatchString(ip) {
		return nil
	}

	return fmt.Errorf("invalid origin IP or hostname format")
}

// ValidateAPIKey validates Bunny API key format
func (v *Validator) ValidateAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Bunny API keys are typically 64 characters (alphanumeric)
	// But we'll be lenient and just check minimum length
	if len(apiKey) < 10 {
		return fmt.Errorf("API key too short (min 10 characters)")
	}

	return nil
}

// ValidateWebhookSecret validates webhook secret
func (v *Validator) ValidateWebhookSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("webhook secret is required")
	}

	// Secret should be at least 16 characters for security
	if len(secret) < 16 {
		return fmt.Errorf("webhook secret too short (min 16 characters)")
	}

	return nil
}

// ValidateMXRecord validates an MX record value
func (v *Validator) ValidateMXRecord(mx string) error {
	if mx == "" {
		return fmt.Errorf("MX record is required")
	}

	// MX records should be hostnames with optional trailing dot
	mx = strings.TrimSuffix(mx, ".")

	if !hostnameRegex.MatchString(mx + ".") {
		return fmt.Errorf("invalid MX record format")
	}

	return nil
}

// ValidateTXTRecord validates a TXT record value
func (v *Validator) ValidateTXTRecord(txt string) error {
	if txt == "" {
		return fmt.Errorf("TXT record is required")
	}

	// TXT records can be any text, but check length
	if len(txt) > 65535 {
		return fmt.Errorf("TXT record too long (max 65535 characters)")
	}

	return nil
}

// CheckDNSRecords checks if DNS records exist for a domain
// Returns map of record type to existence
func (v *Validator) CheckDNSRecords(domain string) (map[string]bool, error) {
	if !v.enableDNSChecks {
		return nil, fmt.Errorf("DNS checks are disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), v.dnsTimeout)
	defer cancel()

	results := make(map[string]bool)

	// Check A record (host lookup)
	_, err := net.DefaultResolver.LookupHost(ctx, domain)
	results["A"] = err == nil

	// Check MX records
	mxRecords, err := net.DefaultResolver.LookupMX(ctx, domain)
	results["MX"] = err == nil && len(mxRecords) > 0

	// Check TXT records
	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, domain)
	results["TXT"] = err == nil && len(txtRecords) > 0

	// Check NS records
	nsRecords, err := net.DefaultResolver.LookupNS(ctx, domain)
	results["NS"] = err == nil && len(nsRecords) > 0

	return results, nil
}
