package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Bunny    BunnyConfig    `mapstructure:"bunny"`
	DNS      DNSConfig      `mapstructure:"dns"`
	CDN      CDNConfig      `mapstructure:"cdn"`
	Origin   OriginConfig   `mapstructure:"origin"`
	Webhook  WebhookConfig  `mapstructure:"webhook"`
	Telegram TelegramConfig `mapstructure:"telegram"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

// BunnyConfig holds Bunny.net API configuration
type BunnyConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

// DNSConfig holds DNS configuration
type DNSConfig struct {
	Nameserver1 string `mapstructure:"nameserver1"`
	Nameserver2 string `mapstructure:"nameserver2"`
	SOAEmail    string `mapstructure:"soa_email"`
}

// CDNConfig holds CDN configuration
type CDNConfig struct {
	OriginShieldRegion string   `mapstructure:"origin_shield_region"`
	Regions            []string `mapstructure:"regions"`
}

// OriginConfig holds origin server configuration
type OriginConfig struct {
	ReverseProxyIP string `mapstructure:"reverse_proxy_ip"`
}

// WebhookConfig holds webhook configuration
type WebhookConfig struct {
	Secret string `mapstructure:"secret"`
}

// TelegramConfig holds Telegram notification configuration
type TelegramConfig struct {
	BotToken string                `mapstructure:"bot_token"`
	ChatID   string                `mapstructure:"chat_id"`
	Enabled  bool                  `mapstructure:"enabled"`
	Events   []string              `mapstructure:"events"`
	Summary  TelegramSummaryConfig `mapstructure:"summary"`
}

// TelegramSummaryConfig holds Telegram daily summary configuration
type TelegramSummaryConfig struct {
	Enabled                 bool   `mapstructure:"enabled"`
	Schedule                string `mapstructure:"schedule"`
	WeeklySchedule          string `mapstructure:"weekly_schedule"`
	Timezone                string `mapstructure:"timezone"`
	IncludeTopBandwidth     int    `mapstructure:"include_top_bandwidth"`
	BandwidthAlertThreshold int    `mapstructure:"bandwidth_alert_threshold"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load loads configuration from file and environment variables
// Environment variables take precedence over file values
// Supported environment variables:
// - BUNNY_API_KEY: Bunny.net API key
// - REVERSE_PROXY_IP: Reverse proxy IP address
// - WHM_HOOK_SECRET: Webhook HMAC secret
// - TELEGRAM_BOT_TOKEN: Telegram bot token (optional)
// - TELEGRAM_CHAT_ID: Telegram chat ID (optional)
func Load(path string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configure file reading
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/whm2bunny/")
		v.AddConfigPath("$HOME/.whm2bunny/")
	}

	// Enable environment variable substitution
	// This allows ${VAR} syntax in config file
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (optional - not required if all env vars are set)
	if path != "" {
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
			// Config file not found; continue with defaults and env vars
		}
	}

	// Unmarshal into config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Substitute environment variables in string values
	substituteEnvVars(&cfg)

	// Override with direct environment variables for common configs
	// This provides a more direct way to set values without needing a config file
	if apiKey := os.Getenv("BUNNY_API_KEY"); apiKey != "" {
		cfg.Bunny.APIKey = apiKey
	}
	if proxyIP := os.Getenv("REVERSE_PROXY_IP"); proxyIP != "" {
		cfg.Origin.ReverseProxyIP = proxyIP
	}
	if secret := os.Getenv("WHM_HOOK_SECRET"); secret != "" {
		cfg.Webhook.Secret = secret
	}
	if botToken := os.Getenv("TELEGRAM_BOT_TOKEN"); botToken != "" {
		cfg.Telegram.BotToken = botToken
	}
	if chatID := os.Getenv("TELEGRAM_CHAT_ID"); chatID != "" {
		cfg.Telegram.ChatID = chatID
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks if all required configuration fields are set
func (c *Config) Validate() error {
	if c.Bunny.APIKey == "" {
		return fmt.Errorf("bunny.api_key is required (set BUNNY_API_KEY env var)")
	}
	if c.Origin.ReverseProxyIP == "" {
		return fmt.Errorf("origin.reverse_proxy_ip is required (set REVERSE_PROXY_IP env var)")
	}
	if c.Webhook.Secret == "" {
		return fmt.Errorf("webhook.secret is required (set WHM_HOOK_SECRET env var)")
	}
	return nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", DefaultPort)
	v.SetDefault("server.host", DefaultHost)

	// Bunny defaults
	v.SetDefault("bunny.base_url", DefaultBunnyBaseURL)

	// DNS defaults
	v.SetDefault("dns.nameserver1", DefaultNameserver1)
	v.SetDefault("dns.nameserver2", DefaultNameserver2)
	v.SetDefault("dns.soa_email", DefaultSOAEmail)

	// CDN defaults
	v.SetDefault("cdn.origin_shield_region", DefaultOriginShieldRegion)
	v.SetDefault("cdn.regions", []string{"asia"})

	// Logging defaults
	v.SetDefault("logging.level", DefaultLogLevel)
	v.SetDefault("logging.format", DefaultLogFormat)

	// Telegram defaults
	v.SetDefault("telegram.enabled", false)
	v.SetDefault("telegram.events", []string{
		"provisioning_success",
		"provisioning_failed",
		"ssl_issued",
		"bandwidth_alert",
		"deprovisioned",
		"subdomain_provisioned",
	})
	v.SetDefault("telegram.summary.enabled", true)
	v.SetDefault("telegram.summary.schedule", "0 9 * * *")
	v.SetDefault("telegram.summary.weekly_schedule", "0 9 * * 1")
	v.SetDefault("telegram.summary.timezone", "Asia/Jakarta")
	v.SetDefault("telegram.summary.include_top_bandwidth", 20)
	v.SetDefault("telegram.summary.bandwidth_alert_threshold", 50)
}

// substituteEnvVars replaces ${VAR} patterns with environment variable values
func substituteEnvVars(cfg *Config) {
	cfg.Bunny.APIKey = envSubstitute(cfg.Bunny.APIKey)
	cfg.Bunny.BaseURL = envSubstitute(cfg.Bunny.BaseURL)
	cfg.Origin.ReverseProxyIP = envSubstitute(cfg.Origin.ReverseProxyIP)
	cfg.Webhook.Secret = envSubstitute(cfg.Webhook.Secret)
	cfg.Telegram.BotToken = envSubstitute(cfg.Telegram.BotToken)
	cfg.Telegram.ChatID = envSubstitute(cfg.Telegram.ChatID)
}

// envSubstitute replaces ${VAR} with the value of the environment variable VAR
// If the environment variable is not set, the ${VAR} pattern is preserved
func envSubstitute(s string) string {
	if s == "" {
		return s
	}

	// Handle inline ${VAR} substitutions within strings
	result := new(strings.Builder)
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			result.WriteString(s)
			break
		}
		end := strings.Index(s, "}")
		if end == -1 || end < start {
			result.WriteString(s)
			break
		}

		// Write the part before ${VAR}
		result.WriteString(s[:start])

		// Extract the variable name
		envVar := s[start+2 : end]

		// Get the value, preserving ${VAR} if not found
		if val := getEnv(envVar, ""); val != "" {
			result.WriteString(val)
		} else {
			result.WriteString(s[start : end+1])
		}

		// Move past this ${VAR}
		s = s[end+1:]
	}

	return result.String()
}

// getEnv retrieves an environment variable or returns the default value
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
