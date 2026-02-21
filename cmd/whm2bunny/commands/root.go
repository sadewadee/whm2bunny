package commands

import "github.com/spf13/cobra"

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "whm2bunny",
	Short: "Auto-provision BunnyDNS Zone + BunnyCDN Pull Zone for WHM/cPanel domains",
	Long: `whm2bunny is a Go daemon that auto-provisions BunnyDNS Zone + BunnyCDN Pull Zone
when a new domain is added to WHM/cPanel. It runs as an HTTP server receiving webhooks
from WHM hooks.`,
}
