package cmd

import (
	"fmt"
	"os"

	"github.com/otiai10/copy"
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
  xplat cp file.txt backup.txt
  xplat cp -r src/ dist/
  xplat cp config.json build/config.json`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		src := args[0]
		dst := args[1]

		info, err := os.Stat(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cp: %s: %v\n", src, err)
			os.Exit(1)
		}

		if info.IsDir() && !cpRecursive {
			fmt.Fprintf(os.Stderr, "cp: %s: is a directory (use -r to copy)\n", src)
			os.Exit(1)
		}

		// Use otiai10/copy for robust cross-platform copying
		opts := copy.Options{
			// Follow symlinks (copy the target, not the link)
			OnSymlink: func(src string) copy.SymlinkAction {
				return copy.Shallow
			},
			// Preserve permissions
			PermissionControl: copy.PerservePermission,
			// Merge directories (don't fail if dst exists)
			OnDirExists: func(src, dst string) copy.DirExistsAction {
				return copy.Merge
			},
		}

		if err := copy.Copy(src, dst, opts); err != nil {
			fmt.Fprintf(os.Stderr, "cp: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	CpCmd.Flags().BoolVarP(&cpRecursive, "recursive", "r", false, "Copy directories recursively")
}
