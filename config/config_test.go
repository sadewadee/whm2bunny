package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Set required env vars
	os.Setenv("BUNNY_API_KEY", "test-api-key")
	os.Setenv("ORIGIN_IP", "192.0.2.1")
	os.Setenv("WHM_HOOK_SECRET", "test-secret")
	defer func() {
		os.Unsetenv("BUNNY_API_KEY")
		os.Unsetenv("ORIGIN_IP")
		os.Unsetenv("WHM_HOOK_SECRET")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check defaults
	if cfg.Server.Port != DefaultPort {
		t.Errorf("Expected Server.Port %d, got %d", DefaultPort, cfg.Server.Port)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Expected Server.Host %s, got %s", DefaultHost, cfg.Server.Host)
	}
	if cfg.Bunny.BaseURL != DefaultBunnyBaseURL {
		t.Errorf("Expected Bunny.BaseURL %s, got %s", DefaultBunnyBaseURL, cfg.Bunny.BaseURL)
	}
	if cfg.DNS.Nameserver1 != DefaultNameserver1 {
		t.Errorf("Expected DNS.Nameserver1 %s, got %s", DefaultNameserver1, cfg.DNS.Nameserver1)
	}
	if cfg.DNS.Nameserver2 != DefaultNameserver2 {
		t.Errorf("Expected DNS.Nameserver2 %s, got %s", DefaultNameserver2, cfg.DNS.Nameserver2)
	}
	if cfg.DNS.SOAEmail != DefaultSOAEmail {
		t.Errorf("Expected DNS.SOAEmail %s, got %s", DefaultSOAEmail, cfg.DNS.SOAEmail)
	}
	if cfg.CDN.OriginShieldRegion != DefaultOriginShieldRegion {
		t.Errorf("Expected CDN.OriginShieldRegion %s, got %s", DefaultOriginShieldRegion, cfg.CDN.OriginShieldRegion)
	}
	if cfg.Logging.Level != DefaultLogLevel {
		t.Errorf("Expected Logging.Level %s, got %s", DefaultLogLevel, cfg.Logging.Level)
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
  host: "0.0.0.0"

bunny:
  api_key: "${BUNNY_API_KEY}"
  base_url: "https://api.bunny.net"

dns:
  nameserver1: "ns1.example.com"
  nameserver2: "ns2.example.com"
  soa_email: "admin@example.com"

cdn:
  origin_shield_region: "LA"
  regions:
    - asia
    - europe

origin:
  ip: "${ORIGIN_IP}"

webhook:
  secret: "${WHM_HOOK_SECRET}"

telegram:
  bot_token: "${TELEGRAM_BOT_TOKEN}"
  chat_id: "${TELEGRAM_CHAT_ID}"
  enabled: true
  events:
    - provisioning_success
    - provisioning_failed
  summary:
    enabled: true
    schedule: "0 8 * * *"
    timezone: "America/New_York"
    include_top_bandwidth: 10
    bandwidth_alert_threshold: 100

logging:
  level: "debug"
  format: "text"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set required env vars
	os.Setenv("BUNNY_API_KEY", "file-api-key")
	os.Setenv("ORIGIN_IP", "203.0.113.1")
	os.Setenv("WHM_HOOK_SECRET", "file-secret")
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456")
	defer func() {
		os.Unsetenv("BUNNY_API_KEY")
		os.Unsetenv("ORIGIN_IP")
		os.Unsetenv("WHM_HOOK_SECRET")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("TELEGRAM_CHAT_ID")
	}()

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check values from file
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected Server.Port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected Server.Host '0.0.0.0', got %s", cfg.Server.Host)
	}
	if cfg.DNS.Nameserver1 != "ns1.example.com" {
		t.Errorf("Expected DNS.Nameserver1 'ns1.example.com', got %s", cfg.DNS.Nameserver1)
	}
	if cfg.DNS.SOAEmail != "admin@example.com" {
		t.Errorf("Expected DNS.SOAEmail 'admin@example.com', got %s", cfg.DNS.SOAEmail)
	}
	if cfg.CDN.OriginShieldRegion != "LA" {
		t.Errorf("Expected CDN.OriginShieldRegion 'LA', got %s", cfg.CDN.OriginShieldRegion)
	}
	if len(cfg.CDN.Regions) != 2 {
		t.Errorf("Expected 2 CDN regions, got %d", len(cfg.CDN.Regions))
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected Logging.Level 'debug', got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Expected Logging.Format 'text', got %s", cfg.Logging.Format)
	}

	// Check env var substitution
	if cfg.Bunny.APIKey != "file-api-key" {
		t.Errorf("Expected Bunny.APIKey 'file-api-key', got %s", cfg.Bunny.APIKey)
	}
	if cfg.Origin.IP != "203.0.113.1" {
		t.Errorf("Expected Origin.IP '203.0.113.1', got %s", cfg.Origin.IP)
	}
	if cfg.Telegram.BotToken != "test-token" {
		t.Errorf("Expected Telegram.BotToken 'test-token', got %s", cfg.Telegram.BotToken)
	}
}

func TestValidateMissingRequired(t *testing.T) {
	tests := []struct {
		name      string
		unsetVars []string
		setVars   map[string]string
		wantErr   string
	}{
		{
			name:      "missing api key",
			unsetVars: []string{},
			setVars: map[string]string{
				"ORIGIN_IP":       "192.0.2.1",
				"WHM_HOOK_SECRET": "test-secret",
			},
			wantErr: "bunny.api_key is required",
		},
		{
			name:      "missing reverse proxy ip",
			unsetVars: []string{},
			setVars: map[string]string{
				"BUNNY_API_KEY":   "test-api-key",
				"WHM_HOOK_SECRET": "test-secret",
			},
			wantErr: "origin.ip is required",
		},
		{
			name:      "missing webhook secret",
			unsetVars: []string{},
			setVars: map[string]string{
				"BUNNY_API_KEY": "test-api-key",
				"ORIGIN_IP":     "192.0.2.1",
			},
			wantErr: "webhook.secret is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env vars
			defer func() {
				os.Unsetenv("BUNNY_API_KEY")
				os.Unsetenv("ORIGIN_IP")
				os.Unsetenv("WHM_HOOK_SECRET")
			}()

			// Unset all first
			os.Unsetenv("BUNNY_API_KEY")
			os.Unsetenv("ORIGIN_IP")
			os.Unsetenv("WHM_HOOK_SECRET")

			// Set only the specified vars
			for k, v := range tt.setVars {
				os.Setenv(k, v)
			}

			_, err := Load("")
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !containsString(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestEnvSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envKey   string
		envVal   string
		expected string
	}{
		{
			name:     "simple substitution",
			input:    "${TEST_VAR}",
			envKey:   "TEST_VAR",
			envVal:   "test-value",
			expected: "test-value",
		},
		{
			name:     "inline substitution",
			input:    "prefix-${TEST_VAR}-suffix",
			envKey:   "TEST_VAR",
			envVal:   "test-value",
			expected: "prefix-test-value-suffix",
		},
		{
			name:     "missing env var - preserve pattern",
			input:    "${MISSING_VAR}",
			envKey:   "OTHER_VAR",
			envVal:   "other-value",
			expected: "${MISSING_VAR}",
		},
		{
			name:     "no substitution",
			input:    "plain-string",
			envKey:   "TEST_VAR",
			envVal:   "test-value",
			expected: "plain-string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.envKey, tt.envVal)
			defer os.Unsetenv(tt.envKey)

			result := envSubstitute(tt.input)
			if result != tt.expected {
				t.Errorf("envSubstitute(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_EXISTS", "value-exists")
	defer os.Unsetenv("TEST_EXISTS")

	tests := []struct {
		name       string
		key        string
		defaultVal string
		expected   string
	}{
		{
			name:       "env var exists",
			key:        "TEST_EXISTS",
			defaultVal: "default",
			expected:   "value-exists",
		},
		{
			name:       "env var missing",
			key:        "TEST_MISSING",
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "empty default",
			key:        "TEST_MISSING",
			defaultVal: "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEnv(tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != DefaultPort {
		t.Errorf("Expected Server.Port %d, got %d", DefaultPort, cfg.Server.Port)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Expected Server.Host %s, got %s", DefaultHost, cfg.Server.Host)
	}
	if cfg.Bunny.BaseURL != DefaultBunnyBaseURL {
		t.Errorf("Expected Bunny.BaseURL %s, got %s", DefaultBunnyBaseURL, cfg.Bunny.BaseURL)
	}
	if cfg.DNS.Nameserver1 != DefaultNameserver1 {
		t.Errorf("Expected DNS.Nameserver1 %s, got %s", DefaultNameserver1, cfg.DNS.Nameserver1)
	}
	if cfg.DNS.Nameserver2 != DefaultNameserver2 {
		t.Errorf("Expected DNS.Nameserver2 %s, got %s", DefaultNameserver2, cfg.DNS.Nameserver2)
	}
	if cfg.DNS.SOAEmail != DefaultSOAEmail {
		t.Errorf("Expected DNS.SOAEmail %s, got %s", DefaultSOAEmail, cfg.DNS.SOAEmail)
	}
	if cfg.CDN.OriginShieldRegion != DefaultOriginShieldRegion {
		t.Errorf("Expected CDN.OriginShieldRegion %s, got %s", DefaultOriginShieldRegion, cfg.CDN.OriginShieldRegion)
	}
	if cfg.Logging.Level != DefaultLogLevel {
		t.Errorf("Expected Logging.Level %s, got %s", DefaultLogLevel, cfg.Logging.Level)
	}
	if cfg.Logging.Format != DefaultLogFormat {
		t.Errorf("Expected Logging.Format %s, got %s", DefaultLogFormat, cfg.Logging.Format)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
