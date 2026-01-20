// Package cmd provides CLI commands for xplat.
//
// process.go - Embedded Process Compose
//
// # CRITICAL: CLI Compatibility Requirement
//
// The `xplat process` command MUST exactly match the upstream process-compose CLI.
// Users must be able to use `xplat process` as a drop-in replacement for
// `process-compose` with identical behavior, flags, and subcommands.
//
// DO NOT:
//   - Add custom flags that conflict with process-compose flags
//   - Modify argument parsing in ways that change process-compose behavior
//   - Intercept or alter subcommands (except for xplat-specific additions like "tools")
//
// The only allowed additions are:
//   - Auto-detection of config files (non-breaking, only when -f not specified)
//   - The "tools" subcommand for xplat-specific tooling (lint, fmt)
//
// # Why Embed Process Compose?
//
// Process Compose is embedded into xplat to create a unified tool for:
//   - Package installation (xplat pkg install)
//   - Binary management (xplat binary install)
//   - Process orchestration (xplat process up/down/logs)
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
// # Commands (v1.87.0)
//
//	xplat process [args...]     # Pass through to process-compose
//	xplat process up            # Start processes (default with TUI)
//	xplat process down          # Stop all processes
//	xplat process graph         # Display dependency graph (NEW in v1.87.0)
//	xplat process logs <name>   # View process logs
//	xplat process list          # List processes and status
//	xplat process restart <n>   # Restart a process
//
// # Key Features (v1.87.0)
//
//   - Dependency Graph: CLI (graph), TUI (Ctrl+Q), API (/graph)
//     Output formats: ascii (default), mermaid, json, yaml
//   - Scheduled Processes: cron and interval-based execution
//     Config: schedule.cron, schedule.interval, schedule.run_on_start
//   - TUI Enhancements: mouse support, Ctrl+Q for graph view
package cmd

import (
	"fmt"
	"os"
	"strings"

	pccmd "github.com/f1bonacc1/process-compose/src/cmd"
	"github.com/joeblew999/xplat/internal/config"
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
	Long: `Manage long-running processes using the embedded process-compose (v1.87.0).

This provides the same functionality as the standalone 'process-compose'
binary, but bundled into xplat for a unified developer experience.

Commands:
  (no subcommand)      Start processes with TUI (default)
  up                   Start all processes
  down                 Stop all running processes
  graph                Display dependency graph (ascii/mermaid/json/yaml)
  logs <process>       View logs for a process
  list                 List all processes with status
  restart <process>    Restart a specific process
  attach               Attach TUI to running server
  info                 Show process-compose info
  recipe               Manage community recipes
  run <process>        Run single process in foreground
  tools                xplat-specific tooling (lint, fmt)

New in v1.87.0:
  - Dependency Graph: visualize process dependencies
    CLI: xplat process graph -f mermaid
    TUI: Press Ctrl+Q to open graph view
    API: GET /graph
  - Scheduled Processes: cron and interval-based execution
    schedule.cron: "0 2 * * *"     # cron expression
    schedule.interval: "30s"        # Go duration
    schedule.run_on_start: true     # run immediately
    schedule.timezone: "UTC"        # for cron
  - TUI: mouse support, configurable escape character

Examples:
  xplat process                        # Start with TUI
  xplat process up hugo                # Start specific process
  xplat process -f custom.yaml         # Use custom config file
  xplat process logs mailerlite        # View logs
  xplat process down                   # Stop all processes
  xplat process list -o wide           # List with details
  xplat process graph                  # ASCII dependency tree
  xplat process graph -f mermaid       # Mermaid diagram for docs
  xplat process graph -f json          # JSON for tooling
  xplat process tools lint             # Lint config files
  xplat process tools fmt              # Format config files

Config files (searched in order):
  - pc.generated.yaml (generated by xplat manifest gen-process)
  - pc.yaml
  - pc.yml
  - process-compose.generated.yaml
  - process-compose.yaml
  - process-compose.yml`,
	DisableFlagParsing: true, // Pass all args through to process-compose
	RunE:               runProcess,
}

func init() {
	// Add xplat-specific subcommands
	ProcessCmd.AddCommand(ProcessDemoCmd)
	ProcessCmd.AddCommand(ProcessToolsCmd)
}

// runProcess is the main entry point for the embedded process-compose.
// It passes all arguments through to process-compose's CLI.
func runProcess(cmd *cobra.Command, args []string) error {
	// Check for xplat-specific subcommands
	if len(args) > 0 {
		switch args[0] {
		case "demo":
			// Handle demo subcommand
			ProcessDemoCmd.SetArgs(args[1:])
			return ProcessDemoCmd.Execute()
		case "tools":
			// Handle tools subcommand
			return ProcessToolsCmd.Execute()
		}
	}
	return runProcessWithArgs(args)
}

// runProcessWithArgs runs process-compose with the given arguments.
func runProcessWithArgs(args []string) error {
	// Handle version flag specially to show our embedded version
	if len(args) == 1 && (args[0] == "-v" || args[0] == "--version" || args[0] == "version") {
		fmt.Printf("process-compose %s (embedded in xplat)\n", processVersion)
		return nil
	}

	// Auto-detect config file if not specified
	args = autoDetectProcessConfig(args)

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

// autoDetectProcessConfig checks if a config file is specified, and if not,
// searches for config files in the priority order defined in config.ProcessComposeSearchOrder().
// Generated files are prioritized over manual files.
// Only applies -f flag for commands that support it (up, or no subcommand).
func autoDetectProcessConfig(args []string) []string {
	// Only apply auto-detect for "up" command or no subcommand (default is up with TUI)
	// Other commands like "list", "down", "attach" connect to running server
	if len(args) > 0 {
		firstArg := args[0]
		// Skip if first arg is a subcommand that doesn't use -f
		if firstArg != "up" && !strings.HasPrefix(firstArg, "-") {
			return args
		}
	}

	// Check if -f or --config is already specified
	for _, arg := range args {
		if arg == "-f" || arg == "--config" {
			return args
		}
		// Check for -f=file or --config=file format
		if len(arg) > 2 && (arg[:2] == "-f" || (len(arg) > 8 && arg[:9] == "--config=")) {
			return args
		}
	}

	// Try config files in priority order
	for _, f := range config.ProcessComposeSearchOrder() {
		if _, err := os.Stat(f); err == nil {
			return append([]string{"-f", f}, args...)
		}
	}

	// No config file found, let process-compose handle it
	return args
}
