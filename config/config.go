package config

import "github.com/spf13/viper"

// Config holds application configuration
type Config struct {
	BunnyAPIKey      string
	ReverseProxyIP   string
	OriginShieldRegion string
	WHMHooksSecret   string
	ServerPort       int
	SOAEmail         string
	TelegramBotToken string
	TelegramChatID   string
}

// Load loads configuration from environment variables and config file
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults()

	// Configure environment variable parsing
	v.SetEnvPrefix("WHM2BUNNY")
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults() {
	// TODO: Set default values
}
