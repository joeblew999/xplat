package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

// RunCmd executes a tool found via xplat which
var RunCmd = &cobra.Command{
	Use:   "run <tool> [args...]",
	Short: "Run a managed tool",
	Long: `Execute a tool found via xplat conventions.

Finds the tool using the same logic as 'xplat which':
  1. Check Taskfiles for install location vars
  2. Fall back to convention locations (~/.bun/bin/, ~/.local/bin/)
  3. Fall back to PATH

This allows users to run any xplat-managed tool without
modifying their shell configuration.

Examples:
  xplat run wrangler --version
  xplat run dummy --help
  xplat run go version`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true, // Pass all flags to the tool
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]
		toolArgs := args[1:]

		// Find the tool using same logic as which (reflects off Taskfiles)
		toolPath := findManagedTool(toolName)
		if toolPath == "" {
			// Fall back to PATH
			var err error
			toolPath, err = exec.LookPath(toolName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tool not found: %s\n", toolName)
				os.Exit(1)
			}
		}

		// Execute the tool, replacing this process
		execArgs := append([]string{toolPath}, toolArgs...)
		if err := syscall.Exec(toolPath, execArgs, os.Environ()); err != nil {
			// syscall.Exec failed, fall back to exec.Command
			execCmd := exec.Command(toolPath, toolArgs...)
			execCmd.Stdin = os.Stdin
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			if err := execCmd.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
				fmt.Fprintf(os.Stderr, "failed to run %s: %v\n", toolName, err)
				os.Exit(1)
			}
		}
	},
}
