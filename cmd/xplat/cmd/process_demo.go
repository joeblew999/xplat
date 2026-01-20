package cmd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed process_demo_fixtures/*
var demoFixtures embed.FS

// ProcessDemoCmd launches process-compose with demo fixtures for testing.
var ProcessDemoCmd = &cobra.Command{
	Use:   "demo [fixture...]",
	Short: "Run demo fixtures to explore process-compose features",
	Long: `Launch process-compose with built-in demo fixtures to explore features.

Available fixtures:
  chain      Linear dependency chain (5 processes)
  diamond    Diamond pattern (web -> api/worker -> db)
  mixed      Production-like setup with services, workers, scheduled jobs
  scheduled  Showcase scheduled processes (cron and interval)
  all        Run ALL fixtures together (demonstrates multi-config)

The Web UI will be available at http://localhost:8761 (process-compose)
You can also run 'xplat up --pc-port 8761' in another terminal to see
the processes in the xplat Web GUI at http://localhost:8760.

Examples:
  xplat process demo                       # List available fixtures
  xplat process demo chain                 # Run chain demo
  xplat process demo diamond               # Run diamond demo
  xplat process demo chain diamond         # Run MULTIPLE fixtures together
  xplat process demo all                   # Run ALL fixtures (multi-config demo)
  xplat process demo mixed --tui=false     # Run without TUI (logs to stdout)`,
	RunE: runProcessDemo,
}

var (
	demoTUI     bool
	demoPort    int
	demoNoGraph bool
)

func init() {
	ProcessDemoCmd.Flags().BoolVar(&demoTUI, "tui", true, "Enable TUI interface")
	ProcessDemoCmd.Flags().IntVar(&demoPort, "port", 8761, "Process-compose API port")
	ProcessDemoCmd.Flags().BoolVar(&demoNoGraph, "no-graph", false, "Don't show graph on startup")
}

// Available demo fixtures
var demoFixtureNames = []string{"chain", "diamond", "mixed", "scheduled"}

func runProcessDemo(cmd *cobra.Command, args []string) error {
	// List fixtures if no argument
	if len(args) == 0 {
		fmt.Println("Available demo fixtures:")
		fmt.Println()
		fixtures := map[string]string{
			"chain":     "Linear dependency chain (step-1 -> step-2 -> ... -> step-5)",
			"diamond":   "Diamond pattern (web -> api/worker -> db)",
			"mixed":     "Production setup: services, workers, scheduled jobs, disabled",
			"scheduled": "Scheduled processes: cron and interval-based execution",
			"all":       "Run ALL fixtures together (multi-config demo)",
		}
		for _, name := range append(demoFixtureNames, "all") {
			fmt.Printf("  %-12s %s\n", name, fixtures[name])
		}
		fmt.Println()
		fmt.Println("Usage: xplat process demo <fixture> [fixture...]")
		fmt.Println()
		fmt.Println("Tips:")
		fmt.Println("  - Specify multiple fixtures to run them together")
		fmt.Println("  - Press Ctrl+Q in TUI to see dependency graph")
		fmt.Println("  - Run 'xplat up' in another terminal to see Web GUI")
		fmt.Println("  - Use --tui=false to see raw log output")
		return nil
	}

	// Handle "all" - expand to all fixtures
	fixtureNames := args
	if len(args) == 1 && args[0] == "all" {
		fixtureNames = demoFixtureNames
	}

	// Validate fixture names
	for _, fixtureName := range fixtureNames {
		valid := false
		for _, name := range demoFixtureNames {
			if name == fixtureName {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown fixture: %s (available: %s, all)", fixtureName, strings.Join(demoFixtureNames, ", "))
		}
	}

	// Create temp directory for config files
	tmpDir, err := os.MkdirTemp("", "xplat-demo-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	// Note: we don't clean up tmpDir so user can inspect if needed

	// Build args for process-compose with multiple -f flags
	pcArgs := []string{}
	var configPaths []string

	for _, fixtureName := range fixtureNames {
		// Read fixture from embedded filesystem
		fixtureFile := fmt.Sprintf("process_demo_fixtures/%s.yaml", fixtureName)
		content, err := demoFixtures.ReadFile(fixtureFile)
		if err != nil {
			return fmt.Errorf("failed to read fixture %s: %w", fixtureName, err)
		}

		// Write to temp file with fixture name
		configPath := filepath.Join(tmpDir, fmt.Sprintf("%s.yaml", fixtureName))
		if err := os.WriteFile(configPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		pcArgs = append(pcArgs, "-f", configPath)
		configPaths = append(configPaths, configPath)
	}

	// Add port and TUI flags
	pcArgs = append(pcArgs, fmt.Sprintf("--port=%d", demoPort))
	if !demoTUI {
		pcArgs = append(pcArgs, "-t=false")
	}

	// Print startup info
	if len(fixtureNames) > 1 {
		fmt.Printf("Starting %d fixtures: %s\n", len(fixtureNames), strings.Join(fixtureNames, ", "))
		fmt.Println("Configs:")
		for _, p := range configPaths {
			fmt.Printf("  - %s\n", p)
		}
	} else {
		fmt.Printf("Starting demo: %s\n", fixtureNames[0])
		fmt.Printf("Config: %s\n", configPaths[0])
	}
	fmt.Printf("API: http://localhost:%d\n", demoPort)
	fmt.Println()
	if demoTUI {
		fmt.Println("TUI Controls:")
		fmt.Println("  Ctrl+Q  - Show dependency graph")
		fmt.Println("  Ctrl+C  - Quit")
		fmt.Println()
	}

	// Run process-compose with all configs
	return runProcessWithArgs(pcArgs)
}
