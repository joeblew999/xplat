package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/taskui"
)

var (
	uiPort       string
	uiTaskfile   string
	uiNoBrowser  bool
)

// UICmd starts the web-based Task UI.
var UICmd = &cobra.Command{
	Use:   "ui",
	Short: "Start web-based Task UI",
	Long: `Starts a web-based interface for running Taskfile tasks.

The UI provides:
- List of available tasks from your Taskfile.yml
- Click-to-run execution with real-time terminal output
- Interactive task support (keyboard input)

Examples:
  xplat ui                    # Start on port 3000
  xplat ui -p 8080            # Start on port 8080
  xplat ui -t Taskfile.ci.yml # Use specific Taskfile
  xplat ui --no-browser       # Don't open browser`,
	RunE: runUI,
}

func init() {
	UICmd.Flags().StringVarP(&uiPort, "port", "p", "3000", "Port to listen on")
	UICmd.Flags().StringVarP(&uiTaskfile, "taskfile", "t", "Taskfile.yml", "Path to Taskfile")
	UICmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "Don't open browser automatically")
}

func runUI(cmd *cobra.Command, args []string) error {
	cfg := taskui.DefaultConfig()
	cfg.ListenAddr = ":" + uiPort
	cfg.Taskfile = uiTaskfile
	cfg.OpenBrowser = !uiNoBrowser

	// Get working directory
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg.WorkDir = wd

	server, err := taskui.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	return server.Start(ctx)
}
