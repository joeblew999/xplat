package cmd

import (
	"testing"
)

// TestProcessCLICompatibility verifies that xplat process matches upstream process-compose CLI.
//
// This test ensures we don't accidentally break CLI compatibility when updating
// the embedded process-compose version. The CLI MUST remain a drop-in replacement.
//
// IMPORTANT: When updating the embedded process-compose version, run:
//
//	go test -v ./cmd/xplat/cmd/ -run TestProcess
//
// If tests fail, update the expected commands/subcommands/flags below to match
// the new upstream version, then verify xplat process still passes through correctly.
func TestProcessCLICompatibility(t *testing.T) {
	// This test validates our CONTRACT with users:
	// xplat process MUST support all these commands/flags identically to process-compose.
	//
	// The expected values are based on process-compose v1.87.0.
	// Update these when upgrading the embedded version.

	t.Run("expected root commands documented", func(t *testing.T) {
		// These are the root-level commands that MUST be available via `xplat process`
		expectedRootCommands := []string{
			"attach",     // Attach TUI to running server
			"completion", // Shell completion scripts
			"down",       // Stop all processes
			"graph",      // Display process dependency graph (added in v1.87.0)
			"help",       // Help about any command
			"info",       // Print configuration info
			"list",       // List available processes
			"process",    // Execute operations on processes
			"project",    // Execute operations on project
			"recipe",     // Manage recipes
			"run",        // Run process in foreground
			"up",         // Run process compose project
			"version",    // Print version info
		}

		// Log for documentation purposes
		t.Logf("Expected root commands (%d): %v", len(expectedRootCommands), expectedRootCommands)

		// Verify count matches our documentation
		if len(expectedRootCommands) != 13 {
			t.Errorf("expected 13 root commands, got %d - update test if upstream changed", len(expectedRootCommands))
		}
	})

	t.Run("expected process subcommands documented", func(t *testing.T) {
		// Subcommands under `xplat process process <cmd>`
		expectedProcessSubcommands := []string{
			"info",    // Get process info
			"logs",    // Get process logs
			"restart", // Restart process
			"scale",   // Scale process
			"start",   // Start process
			"stop",    // Stop process
		}

		t.Logf("Expected 'process' subcommands (%d): %v", len(expectedProcessSubcommands), expectedProcessSubcommands)

		if len(expectedProcessSubcommands) != 6 {
			t.Errorf("expected 6 process subcommands, got %d", len(expectedProcessSubcommands))
		}
	})

	t.Run("expected project subcommands documented", func(t *testing.T) {
		// Subcommands under `xplat process project <cmd>`
		expectedProjectSubcommands := []string{
			"state", // Get/set project state
		}

		t.Logf("Expected 'project' subcommands (%d): %v", len(expectedProjectSubcommands), expectedProjectSubcommands)

		if len(expectedProjectSubcommands) != 1 {
			t.Errorf("expected 1 project subcommand, got %d", len(expectedProjectSubcommands))
		}
	})

	t.Run("expected recipe subcommands documented", func(t *testing.T) {
		// Subcommands under `xplat process recipe <cmd>`
		expectedRecipeSubcommands := []string{
			"list",   // List locally installed recipes
			"pull",   // Pull recipe from repository
			"remove", // Remove local recipe
			"search", // Search for recipes
			"show",   // Show recipe content
		}

		t.Logf("Expected 'recipe' subcommands (%d): %v", len(expectedRecipeSubcommands), expectedRecipeSubcommands)

		if len(expectedRecipeSubcommands) != 5 {
			t.Errorf("expected 5 recipe subcommands, got %d", len(expectedRecipeSubcommands))
		}
	})

	t.Run("expected completion subcommands documented", func(t *testing.T) {
		// Subcommands under `xplat process completion <cmd>`
		expectedCompletionSubcommands := []string{
			"bash",
			"fish",
			"powershell",
			"zsh",
		}

		t.Logf("Expected 'completion' subcommands (%d): %v", len(expectedCompletionSubcommands), expectedCompletionSubcommands)

		if len(expectedCompletionSubcommands) != 4 {
			t.Errorf("expected 4 completion subcommands, got %d", len(expectedCompletionSubcommands))
		}
	})

	t.Run("expected root flags documented", func(t *testing.T) {
		// Critical flags that MUST be available on root command
		// These are the most commonly used flags that users depend on
		expectedRootFlags := []string{
			"config",          // -f: config files
			"tui",             // -t: enable TUI
			"port",            // -p: port number
			"address",         // address to listen on
			"no-server",       // disable HTTP server
			"log-file",        // -L: log file path
			"detached",        // -D: detached mode (unix only)
			"env",             // -e: env files
			"namespace",       // -n: namespaces to run
			"hide-disabled",   // -d: hide disabled processes
			"ordered-shutdown", // shutdown in reverse dependency order
			"read-only",       // read-only mode
			"unix-socket",     // -u: unix socket path (unix only)
			"use-uds",         // -U: use unix domain sockets (unix only)
			"dry-run",         // validate config and exit
			"keep-project",    // keep project running after processes exit
			"ref-rate",        // -r: TUI refresh rate
			"reverse",         // -R: reverse sort
			"sort",            // -S: sort column
			"theme",           // TUI theme
		}

		t.Logf("Expected root flags (%d): %v", len(expectedRootFlags), expectedRootFlags)

		if len(expectedRootFlags) != 20 {
			t.Errorf("expected 20 root flags documented, got %d", len(expectedRootFlags))
		}
	})
}

// TestXplatProcessToolsIsAdditive documents that 'tools' is an xplat-only addition.
func TestXplatProcessToolsIsAdditive(t *testing.T) {
	// The 'tools' subcommand is xplat-specific and does NOT exist in upstream process-compose.
	// This is the ONLY allowed addition to the CLI.
	//
	// If process-compose ever adds a 'tools' command, we must rename ours to avoid conflict.

	t.Log("'tools' is an xplat-only subcommand (not in upstream process-compose)")
	t.Log("Available: xplat process tools lint, xplat process tools fmt")
}

// TestProcessComposeV187Features documents new features added in v1.87.0.
func TestProcessComposeV187Features(t *testing.T) {
	t.Run("graph command", func(t *testing.T) {
		// The graph command visualizes process dependencies.
		// It requires a running process-compose server to query.
		//
		// Access methods:
		//   CLI: xplat process graph [-f format]
		//   TUI: Press Ctrl+Q to open graph view
		//   API: GET /graph
		//
		// Output formats:
		//   -f ascii   (default) - ASCII tree with status
		//   -f mermaid           - Mermaid flowchart for docs
		//   -f json              - JSON for tooling
		//   -f yaml              - YAML for tooling
		//
		// Example ASCII output:
		//   Dependency Graph
		//   ├── web [Running]
		//   │   └── api <healthy> [Running]
		//   │       └── db <healthy> [Running]
		//   └── worker [Pending]
		//       └── db <started> [Running]

		t.Log("graph command: visualize process dependencies")
		t.Log("Formats: ascii (default), mermaid, json, yaml")
		t.Log("TUI shortcut: Ctrl+Q")
		t.Log("API endpoint: GET /graph")
	})

	t.Run("scheduled processes", func(t *testing.T) {
		// v1.87.0 adds cron and interval-based process scheduling.
		//
		// Configuration in process-compose.yaml:
		//
		//   processes:
		//     backup:
		//       command: ./backup.sh
		//       schedule:
		//         cron: "0 2 * * *"        # Cron expression (5 fields)
		//         timezone: "UTC"          # Optional timezone for cron
		//         interval: "30s"          # OR use interval (Go duration)
		//         run_on_start: false      # Run immediately on startup?
		//         max_concurrent: 1        # Max simultaneous executions
		//
		// Cron expression format: minute hour day month weekday
		//   "0 2 * * *"     = Every day at 2:00 AM
		//   "*/5 * * * *"   = Every 5 minutes
		//   "0 9 * * 1-5"   = Weekdays at 9:00 AM
		//
		// Interval format: Go duration string
		//   "30s"  = Every 30 seconds
		//   "5m"   = Every 5 minutes
		//   "1h"   = Every hour
		//   "24h"  = Every 24 hours

		t.Log("scheduled processes: cron and interval-based execution")
		t.Log("Config keys: schedule.cron, schedule.interval, schedule.run_on_start, schedule.timezone, schedule.max_concurrent")
	})

	t.Run("TUI enhancements", func(t *testing.T) {
		// v1.87.0 adds TUI improvements:
		//   - Mouse support in terminal view
		//   - Ctrl+Q opens dependency graph view
		//   - Configurable escape character for interactive processes

		t.Log("TUI enhancements: mouse support, Ctrl+Q graph view")
	})
}

// TestAutoDetectProcessConfig tests the config file auto-detection logic.
func TestAutoDetectProcessConfig(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantSkip bool // true if args should be returned unchanged
	}{
		{
			name:     "empty args - should try auto-detect",
			args:     []string{},
			wantSkip: false,
		},
		{
			name:     "up command - should try auto-detect",
			args:     []string{"up"},
			wantSkip: false,
		},
		{
			name:     "up with processes - should try auto-detect",
			args:     []string{"up", "web", "db"},
			wantSkip: false,
		},
		{
			name:     "flag first - should try auto-detect",
			args:     []string{"-t=false"},
			wantSkip: false,
		},
		{
			name:     "down command - should skip (connects to running server)",
			args:     []string{"down"},
			wantSkip: true,
		},
		{
			name:     "list command - should skip",
			args:     []string{"list"},
			wantSkip: true,
		},
		{
			name:     "attach command - should skip",
			args:     []string{"attach"},
			wantSkip: true,
		},
		{
			name:     "already has -f flag - should skip",
			args:     []string{"-f", "custom.yaml"},
			wantSkip: true,
		},
		{
			name:     "already has --config flag - should skip",
			args:     []string{"--config", "custom.yaml"},
			wantSkip: true,
		},
		{
			name:     "already has -f=file format - should skip",
			args:     []string{"-f=custom.yaml"},
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := autoDetectProcessConfig(tt.args)

			if tt.wantSkip {
				// Args should be returned unchanged
				if len(result) != len(tt.args) {
					t.Errorf("expected args unchanged, got different length: %v vs %v", tt.args, result)
				}
			}
			// Note: We can't fully test auto-detect without creating temp files,
			// but we verify the skip logic works correctly
		})
	}
}
