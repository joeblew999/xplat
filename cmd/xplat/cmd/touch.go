package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// TouchCmd creates or updates file timestamps
var TouchCmd = &cobra.Command{
	Use:   "touch <file>...",
	Short: "Create files or update timestamps",
	Long: `Create empty files or update access/modification times.

Works identically on macOS, Linux, and Windows.
If file doesn't exist, it is created.

Examples:
  xplat touch newfile.txt
  xplat touch file1.txt file2.txt
  xplat touch .timestamp`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hasError := false
		now := time.Now()

		for _, path := range args {
			// Try to update timestamp first
			if err := os.Chtimes(path, now, now); err != nil {
				if os.IsNotExist(err) {
					// Create the file
					f, createErr := os.Create(path)
					if createErr != nil {
						fmt.Fprintf(os.Stderr, "touch: %s: %v\n", path, createErr)
						hasError = true
						continue
					}
					f.Close()
				} else {
					fmt.Fprintf(os.Stderr, "touch: %s: %v\n", path, err)
					hasError = true
				}
			}
		}

		if hasError {
			os.Exit(1)
		}
	},
}
