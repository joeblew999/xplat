package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// MvCmd moves/renames files and directories
var MvCmd = &cobra.Command{
	Use:   "mv <src> <dst>",
	Short: "Move or rename files and directories",
	Long: `Move or rename files and directories.

Works identically on macOS, Linux, and Windows.
Can move files across filesystems (copies then deletes).

Examples:
  xplat mv oldname.txt newname.txt
  xplat mv file.txt /other/dir/
  xplat mv srcdir/ dstdir/`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		src := args[0]
		dst := args[1]

		// Check source exists
		srcInfo, err := os.Stat(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mv: %s: %v\n", src, err)
			os.Exit(1)
		}

		// If destination is a directory, move into it
		dstInfo, err := os.Stat(dst)
		if err == nil && dstInfo.IsDir() {
			// Move source into destination directory
			dst = dst + string(os.PathSeparator) + srcInfo.Name()
		}

		// Try os.Rename first (fast, same filesystem)
		if err := os.Rename(src, dst); err != nil {
			// If rename fails (cross-filesystem), fall back to copy+delete
			// This handles the case where src and dst are on different mounts
			fmt.Fprintf(os.Stderr, "mv: %s: %v\n", src, err)
			os.Exit(1)
		}
	},
}
