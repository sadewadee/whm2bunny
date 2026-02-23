package validator

import (
	"strings"
	"testing"

	"github.com/mordenhost/whm2bunny/internal/webhook"
)

// TestNewValidator tests the Validator constructor
func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil {
		t.Fatal("NewValidator() returned nil")
	}
}

// TestNewValidatorWithConfig tests the Validator constructor with config
func TestNewValidatorWithConfig(t *testing.T) {
	cfg := &ValidatorConfig{
		EnableDNSChecks: false,
		DNSTimeout:      0,
	}
	v := NewValidatorWithConfig(cfg, nil)
	if v == nil {
		t.Fatal("NewValidatorWithConfig() returned nil")
	}
}

// TestValidateDomain_ValidDomains tests valid domain names
func TestValidateDomain_ValidDomains(t *testing.T) {
	v := NewValidator()

	validDomains := []string{
		"example.com",
		"subdomain.example.com",
		"my-domain.com",
		"a.co",
		"test123.com",
		"example.co.uk",
		"sub.domain.example.org",
		"123domain.com",
		"domain.info",
	}

	for _, domain := range validDomains {
		t.Run(domain, func(t *testing.T) {
			err := v.ValidateDomain(domain)
			if err != nil {
				t.Errorf("ValidateDomain(%q) returned error: %v", domain, err)
			}
		})
	}
}

// TestValidateDomain_EmptyString tests empty domain string
func TestValidateDomain_EmptyString(t *testing.T) {
	v := NewValidator()
	err := v.ValidateDomain("")
	if err == nil {
		t.Error("ValidateDomain(\"\") should return error")
	}
}

// TestValidateDomain_Whitespace tests whitespace-only domain
func TestValidateDomain_Whitespace(t *testing.T) {
	v := NewValidator()
	whitespaceDomains := []string{"   ", "\t", "\n"}

	for _, domain := range whitespaceDomains {
		t.Run("whitespace", func(t *testing.T) {
			err := v.ValidateDomain(domain)
			if err == nil {
				t.Errorf("ValidateDomain(%q) should return error", domain)
			}
		})
	}
}

// TestValidateDomain_InvalidCharacters tests domains with invalid characters
func TestValidateDomain_InvalidCharacters(t *testing.T) {
	v := NewValidator()
	invalidDomains := []string{
		"example!.com",
		"example$.com",
		"example space.com",
		"example@.com",
	}

	for _, domain := range invalidDomains {
		t.Run(domain, func(t *testing.T) {
			err := v.ValidateDomain(domain)
			if err == nil {
				t.Errorf("ValidateDomain(%q) should return error", domain)
			}
		})
	}
}

// TestValidateDomain_StartingWithHyphen tests domains starting with hyphen
func TestValidateDomain_StartingWithHyphen(t *testing.T) {
	v := NewValidator()
	invalidDomains := []string{"-example.com", ".example.com"}

	for _, domain := range invalidDomains {
		t.Run(domain, func(t *testing.T) {
			err := v.ValidateDomain(domain)
			if err == nil {
				t.Errorf("ValidateDomain(%q) should return error", domain)
			}
		})
	}
}

// TestValidateDomain_NoTLD tests domains without proper TLD
func TestValidateDomain_NoTLD(t *testing.T) {
	v := NewValidator()
	invalidDomains := []string{"example", "localhost"}

	for _, domain := range invalidDomains {
		t.Run(domain, func(t *testing.T) {
			err := v.ValidateDomain(domain)
			if err == nil {
				t.Errorf("ValidateDomain(%q) should return error", domain)
			}
		})
	}
}

// TestValidateDomain_TooLong tests domain exceeding max length
func TestValidateDomain_TooLong(t *testing.T) {
	v := NewValidator()

	// Label exceeding 63 characters
	longLabel := strings.Repeat("a", 64) + ".com"
	err := v.ValidateDomain(longLabel)
	if err == nil {
		t.Error("ValidateDomain(long label) should return error")
	}

	// Domain exceeding 253 characters total
	longDomain := strings.Repeat("ab.", 85) + "com"
	if len(longDomain) > 253 {
		err = v.ValidateDomain(longDomain)
		if err == nil {
			t.Error("ValidateDomain(long domain) should return error")
		}
	}
}

// TestValidateSubdomain_ValidSubdomains tests valid subdomain labels
func TestValidateSubdomain_ValidLabels(t *testing.T) {
	v := NewValidator()
	validSubdomains := []string{
		"www.example.com",
		"api.example.com",
		"my-sub.example.com",
	}

	for _, sub := range validSubdomains {
		t.Run(sub, func(t *testing.T) {
			err := v.ValidateSubdomain(sub)
			if err != nil {
				t.Errorf("ValidateSubdomain(%q) returned error: %v", sub, err)
			}
		})
	}
}

// TestValidateSubdomain_EmptyString tests empty subdomain
func TestValidateSubdomain_EmptyString(t *testing.T) {
	v := NewValidator()
	err := v.ValidateSubdomain("")
	if err == nil {
		t.Error("ValidateSubdomain(\"\") should return error")
	}
}

// TestValidateSubdomain_NoDot tests subdomain without dot (single label)
func TestValidateSubdomain_NoDot(t *testing.T) {
	v := NewValidator()
	err := v.ValidateSubdomain("www")
	if err == nil {
		t.Error("ValidateSubdomain(\"www\") should return error (needs at least one dot)")
	}
}

// TestValidateWebhookPayload_ValidPayload tests valid payloads
func TestValidateWebhookPayload_ValidPayload(t *testing.T) {
	v := NewValidator()

	payload := &webhook.WebhookPayload{
		Event:  "account_created",
		Domain: "example.com",
		User:   "testuser",
	}

	err := v.ValidateWebhookPayload(payload)
	if err != nil {
		t.Errorf("ValidateWebhookPayload() returned error: %v", err)
	}
}

// TestValidateWebhookPayload_NilPayload tests nil payload
func TestValidateWebhookPayload_NilPayload(t *testing.T) {
	v := NewValidator()
	err := v.ValidateWebhookPayload(nil)
	if err == nil {
		t.Error("ValidateWebhookPayload(nil) should return error")
	}
}

// TestValidateWebhookPayload_InvalidEvent tests invalid event types
func TestValidateWebhookPayload_InvalidEvent(t *testing.T) {
	v := NewValidator()

	payload := &webhook.WebhookPayload{
		Event:  "unknown_event",
		Domain: "example.com",
		User:   "testuser",
	}

	err := v.ValidateWebhookPayload(payload)
	if err == nil {
		t.Error("ValidateWebhookPayload(unknown event) should return error")
	}
}

// TestValidateWebhookPayload_MissingUser tests missing user field
func TestValidateWebhookPayload_MissingUser(t *testing.T) {
	v := NewValidator()

	payload := &webhook.WebhookPayload{
		Event:  "account_created",
		Domain: "example.com",
		User:   "",
	}

	err := v.ValidateWebhookPayload(payload)
	if err == nil {
		t.Error("ValidateWebhookPayload(missing user) should return error")
	}
}

// TestValidateWebhookPayload_MissingDomain tests missing domain
func TestValidateWebhookPayload_MissingDomain(t *testing.T) {
	v := NewValidator()

	payload := &webhook.WebhookPayload{
		Event:  "account_created",
		Domain: "",
		User:   "testuser",
	}

	err := v.ValidateWebhookPayload(payload)
	if err == nil {
		t.Error("ValidateWebhookPayload(missing domain) should return error")
	}
}

// TestValidateWebhookPayload_SubdomainEvent tests subdomain event validation
func TestValidateWebhookPayload_SubdomainEvent(t *testing.T) {
	v := NewValidator()

	// Valid subdomain event
	payload := &webhook.WebhookPayload{
		Event:        "subdomain_created",
		Subdomain:    "www",
		ParentDomain: "example.com",
		User:         "testuser",
	}
	err := v.ValidateWebhookPayload(payload)
	if err != nil {
		t.Errorf("ValidateWebhookPayload(valid subdomain) returned error: %v", err)
	}

	// Missing subdomain
	payload2 := &webhook.WebhookPayload{
		Event:        "subdomain_created",
		Subdomain:    "",
		ParentDomain: "example.com",
		User:         "testuser",
	}
	err = v.ValidateWebhookPayload(payload2)
	if err == nil {
		t.Error("ValidateWebhookPayload(missing subdomain) should return error")
	}

	// Missing parent domain
	payload3 := &webhook.WebhookPayload{
		Event:        "subdomain_created",
		Subdomain:    "www",
		ParentDomain: "",
		User:         "testuser",
	}
	err = v.ValidateWebhookPayload(payload3)
	if err == nil {
		t.Error("ValidateWebhookPayload(missing parent domain) should return error")
	}
}

// TestValidateWebhookPayload_AllEventTypes tests all valid event types
func TestValidateWebhookPayload_AllEventTypes(t *testing.T) {
	v := NewValidator()

	domainEvents := []string{"account_created", "addon_created", "account_deleted"}
	for _, event := range domainEvents {
		t.Run(event, func(t *testing.T) {
			payload := &webhook.WebhookPayload{
				Event:  event,
				Domain: "example.com",
				User:   "testuser",
			}
			err := v.ValidateWebhookPayload(payload)
			if err != nil {
				t.Errorf("ValidateWebhookPayload(event=%s) returned error: %v", event, err)
			}
		})
	}

	// Subdomain event
	t.Run("subdomain_created", func(t *testing.T) {
		payload := &webhook.WebhookPayload{
			Event:        "subdomain_created",
			Subdomain:    "www",
			ParentDomain: "example.com",
			User:         "testuser",
		}
		err := v.ValidateWebhookPayload(payload)
		if err != nil {
			t.Errorf("ValidateWebhookPayload(subdomain_created) returned error: %v", err)
		}
	})
}

// TestValidateOriginIP tests origin IP validation
func TestValidateOriginIP(t *testing.T) {
	v := NewValidator()

	validIPs := []string{"192.168.1.1", "10.0.0.1", "8.8.8.8"}
	for _, ip := range validIPs {
		t.Run("valid_"+ip, func(t *testing.T) {
			err := v.ValidateOriginIP(ip)
			if err != nil {
				t.Errorf("ValidateOriginIP(%q) returned error: %v", ip, err)
			}
		})
	}

	invalidIPs := []string{"", "256.256.256.256", "not-an-ip"}
	for _, ip := range invalidIPs {
		t.Run("invalid_"+ip, func(t *testing.T) {
			err := v.ValidateOriginIP(ip)
			if err == nil {
				t.Errorf("ValidateOriginIP(%q) should return error", ip)
			}
		})
	}
}

// BenchmarkValidateDomain benchmarks domain validation
func BenchmarkValidateDomain(b *testing.B) {
	v := NewValidator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.ValidateDomain("example.com")
	}
}
