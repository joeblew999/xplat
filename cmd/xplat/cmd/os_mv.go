package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
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

		if err := osutil.Move(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "mv: %s: %v\n", src, err)
			os.Exit(1)
		}
	},
}
