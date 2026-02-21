package commands

import "github.com/spf13/cobra"

// ServeCmd starts the HTTP server to receive webhooks from WHM
var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the webhook HTTP server",
	Long:  "Start the HTTP server that listens for webhooks from WHM/cPanel",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement HTTP server
		return nil
	},
}
