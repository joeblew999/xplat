package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
	"github.com/spf13/cobra"
)

var mkdirParents bool

// MkdirCmd creates directories
var MkdirCmd = &cobra.Command{
	Use:   "mkdir <path>...",
	Short: "Create directories",
	Long: `Create directories.

Works identically on macOS, Linux, and Windows.

Flags:
  -p, --parents  Create parent directories as needed, no error if existing

Examples:
  xplat mkdir build
  xplat mkdir -p src/components/ui
  xplat mkdir -p dist/ tmp/`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hasError := false

		for _, path := range args {
			if err := osutil.Mkdir(path, mkdirParents); err != nil {
				fmt.Fprintf(os.Stderr, "mkdir: %s: %v\n", path, err)
				hasError = true
			}
		}

		if hasError {
			os.Exit(1)
		}
	},
}

func init() {
	MkdirCmd.Flags().BoolVarP(&mkdirParents, "parents", "p", false, "Create parent directories as needed")
}
