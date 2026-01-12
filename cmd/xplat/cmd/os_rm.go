package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
	"github.com/spf13/cobra"
)

var (
	rmRecursive bool
	rmForce     bool
)

// RmCmd removes files and directories
var RmCmd = &cobra.Command{
	Use:   "rm <path>...",
	Short: "Remove files or directories",
	Long: `Remove files or directories.

Works identically on macOS, Linux, and Windows.

Flags:
  -r, --recursive  Remove directories and their contents recursively
  -f, --force      Ignore nonexistent files, never prompt

Examples:
  xplat os rm file.txt
  xplat os rm -rf build/
  xplat os rm -rf dist/ node_modules/`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hasError := false

		for _, path := range args {
			if err := osutil.Remove(path, rmRecursive, rmForce); err != nil {
				fmt.Fprintf(os.Stderr, "rm: %s: %v\n", path, err)
				hasError = true
			}
		}

		if hasError {
			os.Exit(1)
		}
	},
}

func init() {
	RmCmd.Flags().BoolVarP(&rmRecursive, "recursive", "r", false, "Remove directories recursively")
	RmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Ignore nonexistent files")
}
