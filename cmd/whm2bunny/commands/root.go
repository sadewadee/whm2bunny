package commands

import (
	"github.com/spf13/cobra"
)

var (
	// cfgFile is the path to the configuration file
	cfgFile string
	// verbose enables verbose output
	verbose bool
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "whm2bunny",
	Short: "Auto-provision BunnyDNS Zone + BunnyCDN Pull Zone for WHM/cPanel domains",
	Long: `whm2bunny is a Go daemon that auto-provisions BunnyDNS Zone + BunnyCDN Pull Zone
when a new domain is added to WHM/cPanel. It runs as an HTTP server receiving webhooks
from WHM hooks.`,
}

// Execute runs the root command
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "/etc/whm2bunny/config.yaml", "config file path")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}
