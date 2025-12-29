package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
	"github.com/spf13/cobra"
)

// GlobCmd expands glob patterns
var GlobCmd = &cobra.Command{
	Use:   "glob <pattern>",
	Short: "Expand glob pattern",
	Long: `Expand a glob pattern and print matching files.

Supports doublestar (**) patterns for recursive matching.
On Windows, matching is case-insensitive by default.
Works with both relative and absolute paths.

Patterns:
  *        - matches any sequence of characters (not including /)
  **       - matches any sequence including /
  ?        - matches any single character
  [abc]    - matches one of the characters
  {a,b}    - matches either 'a' or 'b'

Examples:
  xplat glob "taskfiles/*.yml"
  xplat glob "taskfiles/Taskfile.*.yml"
  xplat glob "**/*.go"
  xplat glob "src/**/*.{ts,tsx}"
  xplat glob "/absolute/path/**/*.txt"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pattern := args[0]

		matches, err := osutil.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, match := range matches {
			fmt.Println(match)
		}
	},
}
