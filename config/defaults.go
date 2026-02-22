package config

const (
	// DefaultPort is the default HTTP server port
	DefaultPort = 9090

	// DefaultHost is the default HTTP server host
	DefaultHost = "127.0.0.1"

	// DefaultBunnyBaseURL is the default Bunny.net API base URL
	DefaultBunnyBaseURL = "https://api.bunny.net"

	// DefaultNameserver1 is the default primary nameserver
	DefaultNameserver1 = "ns1.mordenhost.com"

	// DefaultNameserver2 is the default secondary nameserver
	DefaultNameserver2 = "ns2.mordenhost.com"

	// DefaultSOAEmail is the default SOA contact email
	DefaultSOAEmail = "hostmaster@mordenhost.com"

	// DefaultOriginShieldRegion is the default CDN origin shield region
	DefaultOriginShieldRegion = "SG"

	// DefaultLogLevel is the default logging level
	DefaultLogLevel = "info"

	// DefaultLogFormat is the default log format (json or text)
	DefaultLogFormat = "json"
)

// Defaults returns a Config struct with all default values set
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: DefaultPort,
			Host: DefaultHost,
		},
		Bunny: BunnyConfig{
			BaseURL: DefaultBunnyBaseURL,
		},
		DNS: DNSConfig{
			Nameserver1: DefaultNameserver1,
			Nameserver2: DefaultNameserver2,
			SOAEmail:    DefaultSOAEmail,
		},
		CDN: CDNConfig{
			OriginShieldRegion: DefaultOriginShieldRegion,
			Regions:            []string{"asia"},
		},
		Telegram: TelegramConfig{
			Enabled: false,
			Events: []string{
				"provisioning_success",
				"provisioning_failed",
				"ssl_issued",
				"bandwidth_alert",
				"deprovisioned",
				"subdomain_provisioned",
			},
			Summary: TelegramSummaryConfig{
				Enabled:                 true,
				Schedule:                "0 9 * * *",
				WeeklySchedule:          "0 9 * * 1",
				Timezone:                "Asia/Jakarta",
				IncludeTopBandwidth:     20,
				BandwidthAlertThreshold: 50,
			},
		},
		Logging: LoggingConfig{
			Level:  DefaultLogLevel,
			Format: DefaultLogFormat,
		},
	}
}
