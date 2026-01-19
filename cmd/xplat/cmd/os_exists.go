package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// ExistsCmd checks if a file or directory exists
var ExistsCmd = &cobra.Command{
	Use:   "exists <path>",
	Short: "Check if file or directory exists",
	Long: `Check if a file or directory exists.

Works identically on macOS, Linux, and Windows.

Exit codes:
  0 - Path exists
  1 - Path does not exist or error

Examples:
  xplat os exists file.txt
  xplat os exists .src/polyform/.git
  xplat os exists build/ && echo "build exists"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		if _, err := os.Stat(path); err != nil {
			os.Exit(1)
		}
		// Path exists, exit 0 (success)
	},
}
