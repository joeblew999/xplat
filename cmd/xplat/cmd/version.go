package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersion sets the version string (called from main)
func SetVersion(v string) {
	version = v
}

// VersionCmd prints the version
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print xplat version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}
