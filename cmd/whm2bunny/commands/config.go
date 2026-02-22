package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mordenhost/whm2bunny/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ConfigCmd handles configuration management
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  "View and validate application configuration",
}

var configGenerateCmd = &cobra.Command{
	Use:   "generate [output-file]",
	Short: "Generate default config file",
	Long:  "Generate a default configuration file to the specified path",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigGenerate,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate configuration file",
	Long:  "Validate the specified configuration file",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigValidate,
}

var configShowCmd = &cobra.Command{
	Use:   "show [config-file]",
	Short: "Show current configuration",
	Long:  "Show the configuration that will be used (merges defaults, file, and env vars)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigShow,
}

func init() {
	RootCmd.AddCommand(ConfigCmd)
	ConfigCmd.AddCommand(configGenerateCmd)
	ConfigCmd.AddCommand(configValidateCmd)
	ConfigCmd.AddCommand(configShowCmd)
}

func runConfigGenerate(cmd *cobra.Command, args []string) error {
	// Determine output path
	outputPath := "config.yaml"
	if len(args) > 0 {
		outputPath = args[0]
	} else if cfgFile != "" {
		outputPath = cfgFile
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		// File exists
		fmt.Printf("File already exists: %s\n", outputPath)
		fmt.Print("Overwrite? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return nil
		}
	}

	// Generate default config
	cfg := config.Defaults()

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := "# whm2bunny Configuration File\n" +
		"#\n" +
		"# Environment variables can be used as values with ${VAR} syntax\n" +
		"#\n" +
		"# Required environment variables:\n" +
		"#   BUNNY_API_KEY - Bunny.net API key\n" +
		"#   REVERSE_PROXY_IP - IP address of reverse proxy\n" +
		"#   WHM_HOOK_SECRET - Secret for webhook HMAC verification\n" +
		"#\n" +
		"# Optional environment variables:\n" +
		"#   TELEGRAM_BOT_TOKEN - Telegram bot token for notifications\n" +
		"#   TELEGRAM_CHAT_ID - Telegram chat ID for notifications\n" +
		"#\n"

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(outputPath, append([]byte(header), data...), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Generated config file: %s\n", outputPath)
	fmt.Println("\nEdit the file to set your configuration.")
	fmt.Println("Required fields can also be set via environment variables.")

	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	// Determine config path
	configPath := cfgFile
	if len(args) > 0 {
		configPath = args[0]
	}

	// Try to load the config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Validation FAILED: %v\n", err)
		return err
	}

	fmt.Println("Validation PASSED")
	fmt.Printf("Config loaded from: %s\n", configPath)
	fmt.Printf("Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Bunny API: %s\n", cfg.Bunny.BaseURL)
	fmt.Printf("Nameservers: %s, %s\n", cfg.DNS.Nameserver1, cfg.DNS.Nameserver2)
	fmt.Printf("Telegram: %v\n", cfg.Telegram.Enabled)

	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	// Determine config path
	configPath := cfgFile
	if len(args) > 0 {
		configPath = args[0]
	}

	// Load the config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Display current configuration (masking sensitive values)
	fmt.Println("Current Configuration:")
	fmt.Println("=====================")
	fmt.Printf("Server:\n")
	fmt.Printf("  Host: %s\n", cfg.Server.Host)
	fmt.Printf("  Port: %d\n", cfg.Server.Port)
	fmt.Printf("\nBunny:\n")
	fmt.Printf("  API Key: %s\n", maskSensitive(cfg.Bunny.APIKey))
	fmt.Printf("  Base URL: %s\n", cfg.Bunny.BaseURL)
	fmt.Printf("\nDNS:\n")
	fmt.Printf("  Nameserver 1: %s\n", cfg.DNS.Nameserver1)
	fmt.Printf("  Nameserver 2: %s\n", cfg.DNS.Nameserver2)
	fmt.Printf("  SOA Email: %s\n", cfg.DNS.SOAEmail)
	fmt.Printf("\nCDN:\n")
	fmt.Printf("  Origin Shield Region: %s\n", cfg.CDN.OriginShieldRegion)
	fmt.Printf("  Regions: %v\n", cfg.CDN.Regions)
	fmt.Printf("\nOrigin:\n")
	fmt.Printf("  Reverse Proxy IP: %s\n", cfg.Origin.ReverseProxyIP)
	fmt.Printf("\nWebhook:\n")
	fmt.Printf("  Secret: %s\n", maskSensitive(cfg.Webhook.Secret))
	fmt.Printf("\nTelegram:\n")
	fmt.Printf("  Enabled: %v\n", cfg.Telegram.Enabled)
	fmt.Printf("  Bot Token: %s\n", maskSensitive(cfg.Telegram.BotToken))
	fmt.Printf("  Chat ID: %s\n", cfg.Telegram.ChatID)
	fmt.Printf("  Events: %v\n", cfg.Telegram.Events)
	fmt.Printf("\nLogging:\n")
	fmt.Printf("  Level: %s\n", cfg.Logging.Level)
	fmt.Printf("  Format: %s\n", cfg.Logging.Format)

	return nil
}

// maskSensitive masks sensitive configuration values
func maskSensitive(value string) string {
	if value == "" {
		return "(not set)"
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}
