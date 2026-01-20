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
	Use:   "demo [fixture]",
	Short: "Run demo fixtures to explore process-compose features",
	Long: `Launch process-compose with built-in demo fixtures to explore features.

Available fixtures:
  chain      Linear dependency chain (5 processes)
  diamond    Diamond pattern (web -> api/worker -> db)
  mixed      Production-like setup with services, workers, scheduled jobs
  scheduled  Showcase scheduled processes (cron and interval)

The Web UI will be available at http://localhost:8761 (process-compose)
You can also run 'xplat up --pc-port 8761' in another terminal to see
the processes in the xplat Web GUI at http://localhost:8760.

Examples:
  xplat process demo                    # List available fixtures
  xplat process demo chain              # Run chain demo
  xplat process demo diamond            # Run diamond demo
  xplat process demo mixed              # Run mixed demo with scheduled jobs
  xplat process demo scheduled          # Run scheduled processes demo
  xplat process demo chain --tui=false  # Run without TUI (logs to stdout)`,
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
		}
		for _, name := range demoFixtureNames {
			fmt.Printf("  %-12s %s\n", name, fixtures[name])
		}
		fmt.Println()
		fmt.Println("Usage: xplat process demo <fixture>")
		fmt.Println()
		fmt.Println("Tips:")
		fmt.Println("  - Press Ctrl+Q in TUI to see dependency graph")
		fmt.Println("  - Run 'xplat up' in another terminal to see Web GUI")
		fmt.Println("  - Use --tui=false to see raw log output")
		return nil
	}

	fixtureName := args[0]

	// Validate fixture name
	valid := false
	for _, name := range demoFixtureNames {
		if name == fixtureName {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown fixture: %s (available: %s)", fixtureName, strings.Join(demoFixtureNames, ", "))
	}

	// Read fixture from embedded filesystem
	fixtureFile := fmt.Sprintf("process_demo_fixtures/%s.yaml", fixtureName)
	content, err := demoFixtures.ReadFile(fixtureFile)
	if err != nil {
		return fmt.Errorf("failed to read fixture %s: %w", fixtureName, err)
	}

	// Write to temp file
	tmpDir, err := os.MkdirTemp("", "xplat-demo-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	// Note: we don't clean up tmpDir so user can inspect if needed

	configPath := filepath.Join(tmpDir, "process-compose.yaml")
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Starting demo: %s\n", fixtureName)
	fmt.Printf("Config: %s\n", configPath)
	fmt.Printf("API: http://localhost:%d\n", demoPort)
	fmt.Println()
	if demoTUI {
		fmt.Println("TUI Controls:")
		fmt.Println("  Ctrl+Q  - Show dependency graph")
		fmt.Println("  Ctrl+C  - Quit")
		fmt.Println()
	}

	// Build args for process-compose
	pcArgs := []string{"-f", configPath, fmt.Sprintf("--port=%d", demoPort)}

	if !demoTUI {
		pcArgs = append(pcArgs, "-t=false")
	}

	// Run process-compose
	return runProcessWithArgs(pcArgs)
}
