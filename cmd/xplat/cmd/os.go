package cmd

import (
	"github.com/spf13/cobra"
)

// OsCmd is the parent command for cross-platform OS utilities.
var OsCmd = &cobra.Command{
	Use:   "os",
	Short: "Cross-platform OS utilities",
	Long: `Cross-platform OS utilities that work identically on macOS, Linux, and Windows.

These utilities are designed to fill gaps in Task's built-in shell interpreter
and provide consistent behavior across all platforms.

File Operations:
  cat      - Print file contents
  cp       - Copy files or directories
  mkdir    - Create directories
  mv       - Move or rename files and directories
  rm       - Remove files or directories
  touch    - Create files or update timestamps

Environment & Text:
  env      - Get environment variable
  envsubst - Substitute environment variables in text
  glob     - Expand glob pattern
  jq       - Process JSON with jq syntax

Version Control:
  git      - Git operations (no git binary required)

Archives & Downloads:
  extract  - Extract archives (zip, tar.gz, etc.)
  fetch    - Download files with optional extraction

Tools:
  which    - Find binary in managed locations or PATH
  version-file - Read/write .version file

Examples:
  xplat os cat file.txt
  xplat os cp src dst -r
  xplat os envsubst --env-file .env template.yml
  xplat os glob "**/*.go"
  xplat os which go
  xplat os fetch https://example.com/file.tar.gz`,
}

func init() {
	// Add all OS utility commands as subcommands
	OsCmd.AddCommand(CatCmd)
	OsCmd.AddCommand(CpCmd)
	OsCmd.AddCommand(EnvCmd)
	OsCmd.AddCommand(EnvsubstCmd)
	OsCmd.AddCommand(ExtractCmd)
	OsCmd.AddCommand(FetchCmd)
	OsCmd.AddCommand(GitCmd)
	OsCmd.AddCommand(GlobCmd)
	OsCmd.AddCommand(JqCmd)
	OsCmd.AddCommand(MkdirCmd)
	OsCmd.AddCommand(MvCmd)
	OsCmd.AddCommand(RmCmd)
	OsCmd.AddCommand(TouchCmd)
	OsCmd.AddCommand(VersionFileCmd)
	OsCmd.AddCommand(WhichCmd)
}
