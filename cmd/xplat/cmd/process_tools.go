package cmd

import (
	"github.com/spf13/cobra"
)

// ProcessToolsCmd groups xplat-specific process-compose tooling commands.
var ProcessToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Process-compose validation and formatting tools",
	Long: `xplat-specific tools for validating and formatting process-compose files.

Commands:
  lint       Lint process-compose files for xplat conventions
  fmt        Format process-compose files with auto-fixes
  graph      Generate dependency graph from config (offline)

Examples:
  xplat process tools lint                 # Lint process-compose files
  xplat process tools fmt                  # Format process-compose files
  xplat process tools lint --strict        # Treat warnings as errors
  xplat process tools graph -f mermaid     # Generate Mermaid diagram for docs`,
}

func init() {
	// Add all tooling subcommands
	ProcessToolsCmd.AddCommand(ProcessLintCmd)
	ProcessToolsCmd.AddCommand(ProcessFmtCmd)
	ProcessToolsCmd.AddCommand(ProcessGraphCmd)
}
