package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
	"github.com/spf13/cobra"
)

// CatCmd prints file contents
var CatCmd = &cobra.Command{
	Use:   "cat <file>...",
	Short: "Print file contents",
	Long: `Print file contents to stdout.

Works identically on macOS, Linux, and Windows.
Multiple files are concatenated.

Examples:
  xplat os cat file.txt
  xplat os cat header.txt body.txt footer.txt`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hasError := false

		for _, path := range args {
			if err := osutil.CatToWriter(path, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", path, err)
				hasError = true
			}
		}

		if hasError {
			os.Exit(1)
		}
	},
}
