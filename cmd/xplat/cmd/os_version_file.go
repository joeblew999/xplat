package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var versionFilePath string
var versionFileSet string

// VersionFileCmd reads or writes version from a .version file
var VersionFileCmd = &cobra.Command{
	Use:   "version-file",
	Short: "Read or write .version file",
	Long: `Read or write version string from a .version file.

Works identically on macOS, Linux, and Windows.
Returns "dev" if file doesn't exist (when reading).
No git required - pure file-based versioning.

Use this in Taskfiles for build-time version injection.

Flags:
  -f, --file  Path to version file (default: .version)
  -s, --set   Write this version to the file

Examples:
  xplat version-file                    # Read .version (prints "dev" if missing)
  xplat version-file -s v1.0.0          # Write v1.0.0 to .version
  xplat version-file -f VERSION -s 2.0  # Write 2.0 to VERSION file

Taskfile usage:
  build:
    cmds:
      - xplat version-file -s {{.VERSION}}
      - go build -ldflags="-X main.Version=$(xplat version-file)"`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path := versionFilePath
		if path == "" {
			path = ".version"
		}

		// Write mode
		if versionFileSet != "" {
			version := strings.TrimSpace(versionFileSet)
			if err := os.WriteFile(path, []byte(version+"\n"), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "version-file: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(version)
			return
		}

		// Read mode
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("dev")
				return
			}
			fmt.Fprintf(os.Stderr, "version-file: %v\n", err)
			os.Exit(1)
		}

		version := strings.TrimSpace(string(data))
		if version == "" {
			version = "dev"
		}
		fmt.Println(version)
	},
}

func init() {
	VersionFileCmd.Flags().StringVarP(&versionFilePath, "file", "f", "", "Path to version file (default: .version)")
	VersionFileCmd.Flags().StringVarP(&versionFileSet, "set", "s", "", "Write this version to the file")
}
