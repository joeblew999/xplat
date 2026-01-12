package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/osutil"
	"github.com/spf13/cobra"
)

var envDefault string

// EnvCmd gets environment variables
var EnvCmd = &cobra.Command{
	Use:   "env <key>",
	Short: "Get environment variable",
	Long: `Get an environment variable value.

Works identically on macOS, Linux, and Windows.
Returns exit code 1 if variable is not set (unless -d default provided).

Flags:
  -d, --default  Default value if variable is not set

Examples:
  xplat os env HOME
  xplat os env PATH
  xplat os env MY_VAR -d "default_value"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if !osutil.EnvExists(key) {
			if envDefault != "" {
				fmt.Println(envDefault)
				return
			}
			os.Exit(1)
		}

		fmt.Println(osutil.Env(key))
	},
}

func init() {
	EnvCmd.Flags().StringVarP(&envDefault, "default", "d", "", "Default value if not set")
}
