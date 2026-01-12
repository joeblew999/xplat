package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
	"github.com/spf13/cobra"
)

var cpRecursive bool

// CpCmd copies files and directories
var CpCmd = &cobra.Command{
	Use:   "cp <src> <dst>",
	Short: "Copy files or directories",
	Long: `Copy files or directories.

Works identically on macOS, Linux, and Windows.
Handles symlinks, permissions, and directory merging correctly.

Flags:
  -r, --recursive  Copy directories recursively

Examples:
  xplat os cp file.txt backup.txt
  xplat os cp -r src/ dist/
  xplat os cp config.json build/config.json`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		src := args[0]
		dst := args[1]

		if err := osutil.Copy(src, dst, cpRecursive); err != nil {
			fmt.Fprintf(os.Stderr, "cp: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	CpCmd.Flags().BoolVarP(&cpRecursive, "recursive", "r", false, "Copy directories recursively")
}
