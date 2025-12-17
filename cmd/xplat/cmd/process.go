// Package cmd provides CLI commands for xplat.
//
// process.go - Embedded Process Compose
//
// # Why Embed Process Compose?
//
// Process Compose is embedded into xplat to create a unified tool for:
// - Package installation (xplat pkg install)
// - Binary management (xplat binary install)
// - Process orchestration (xplat process up/down/logs)
//
// This eliminates the need for a separate process-compose binary and allows
// for future integration between the package registry and process management.
//
// # How It Works
//
// Since process-compose's cmd package exposes an Execute() function that
// handles the entire CLI, we simply wrap it and pass through arguments.
// This provides full CLI compatibility with standalone process-compose.
//
// # Commands
//
//	xplat process [args...]     # Pass through to process-compose
//	xplat process up            # Start processes (default with TUI)
//	xplat process down          # Stop all processes
//	xplat process logs <name>   # View process logs
//	xplat process list          # List processes and status
//	xplat process restart <n>   # Restart a process
package cmd

import (
	"fmt"
	"os"

	pccmd "github.com/f1bonacc1/process-compose/src/cmd"
	"github.com/spf13/cobra"
)

// processVersion is set via ldflags at build time
var processVersion = "embedded"

// ProcessCmd embeds the Process Compose orchestrator into xplat.
//
// Usage:
//
//	xplat process [flags] [command] [args...]
//
// This is designed to be a drop-in replacement for the standalone
// process-compose binary. All flags and subcommands are passed through.
var ProcessCmd = &cobra.Command{
	Use:   "process [flags] [command] [args...]",
	Short: "Process orchestration (embedded process-compose)",
	Long: `Manage long-running processes using the embedded process-compose.

This provides the same functionality as the standalone 'process-compose'
binary, but bundled into xplat for a unified developer experience.

Commands:
  (no subcommand)      Start processes with TUI (default)
  up                   Start all processes
  down                 Stop all running processes
  logs <process>       View logs for a process
  list                 List all processes with status
  restart <process>    Restart a specific process
  attach <process>     Attach to a running process
  info                 Show process-compose info

Examples:
  xplat process                    # Start with TUI
  xplat process up hugo            # Start specific process
  xplat process -f custom.yaml     # Use custom config file
  xplat process logs mailerlite    # View logs
  xplat process down               # Stop all processes
  xplat process list -o wide       # List with details

Config files (searched in order):
  - process-compose.yaml
  - process-compose.yml
  - compose.yaml
  - compose.yml`,
	DisableFlagParsing: true, // Pass all args through to process-compose
	RunE:               runProcess,
}

// DevCmd provides shortcuts for common dev workflow commands.
// These are aliases for process subcommands with sensible defaults.
var DevCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development workflow shortcuts",
	Long: `Shortcuts for common development workflow commands.

These are convenient aliases for process-compose operations:
  dev up     = process (start with TUI)
  dev down   = process down
  dev logs   = process logs -f
  dev status = process list -o wide`,
}

var devUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start dev environment (process-compose with TUI)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProcessWithArgs(args)
	},
}

var devDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop dev environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProcessWithArgs(append([]string{"down"}, args...))
	},
}

var devLogsCmd = &cobra.Command{
	Use:   "logs [process]",
	Short: "Follow dev logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		pcArgs := []string{"process", "logs", "-f"}
		return runProcessWithArgs(append(pcArgs, args...))
	},
}

var devStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show dev environment status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProcessWithArgs([]string{"list", "-o", "wide"})
	},
}

func init() {
	DevCmd.AddCommand(devUpCmd)
	DevCmd.AddCommand(devDownCmd)
	DevCmd.AddCommand(devLogsCmd)
	DevCmd.AddCommand(devStatusCmd)
}

// runProcess is the main entry point for the embedded process-compose.
// It passes all arguments through to process-compose's CLI.
func runProcess(cmd *cobra.Command, args []string) error {
	return runProcessWithArgs(args)
}

// runProcessWithArgs runs process-compose with the given arguments.
func runProcessWithArgs(args []string) error {
	// Handle version flag specially to show our embedded version
	if len(args) == 1 && (args[0] == "-v" || args[0] == "--version" || args[0] == "version") {
		fmt.Printf("process-compose %s (embedded in xplat)\n", processVersion)
		return nil
	}

	// Save original args and restore after
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Set up args for process-compose
	// process-compose expects os.Args[0] to be the program name
	os.Args = append([]string{"process-compose"}, args...)

	// Execute process-compose
	// Note: Execute() calls os.Exit on error, so we need to handle this
	// Unfortunately process-compose doesn't expose a non-exiting API,
	// so errors result in program exit
	pccmd.Execute()

	return nil
}
