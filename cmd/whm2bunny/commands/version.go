package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version is the application version
	Version = "dev"
	// Commit is the git commit hash
	Commit = "unknown"
	// BuildTime is the build timestamp
	BuildTime = "unknown"
)

// VersionCmd displays version information
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("whm2bunny %s\n", Version)
		fmt.Printf("Commit: %s\n", Commit)
		fmt.Printf("Built at: %s\n", BuildTime)
	},
}
