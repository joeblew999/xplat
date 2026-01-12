package cmd

import (
	"github.com/spf13/cobra"
)

// TaskToolsCmd groups xplat-specific Taskfile tooling commands.
var TaskToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Taskfile validation and formatting tools",
	Long: `xplat-specific tools for validating and formatting Taskfiles.

Commands:
  lint       Lint Taskfiles for cross-platform compatibility
  fmt        Format Taskfiles with auto-fixes
  test       Run Taskfile tests
  archetypes List available archetypes
  detect     Detect archetype of a Taskfile
  explain    Explain archetype requirements

Examples:
  xplat task tools lint                    # Lint all taskfiles
  xplat task tools fmt                     # Format all taskfiles
  xplat task tools detect Taskfile.yml     # Detect archetype`,
}

func init() {
	// Add all tooling subcommands
	TaskToolsCmd.AddCommand(FmtCmd)
	TaskToolsCmd.AddCommand(LintCmd)
	TaskToolsCmd.AddCommand(TestCmd)
	TaskToolsCmd.AddCommand(taskfileArchetypesCmd)
	TaskToolsCmd.AddCommand(taskfileDetectCmd)
	TaskToolsCmd.AddCommand(taskfileExplainCmd)
}
