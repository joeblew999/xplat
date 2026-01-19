package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	web "github.com/joeblew999/xplat/internal/webui"
)

var uiPort string
var uiNoBrowser bool
var uiTaskfile string
var uiDir string
var uiPCPort int

// UICmd starts the Task UI web interface.
// Deprecated: Use 'xplat up' instead for the unified experience.
var UICmd = &cobra.Command{
	Use:        "ui",
	Short:      "Start Task UI web interface (use 'xplat up' for unified UI)",
	Deprecated: "use 'xplat up' for the unified xplat UI with tasks, processes, and setup",
	Long: `Start a web-based UI for running Taskfile tasks.

DEPRECATED: This command is deprecated. Use 'xplat up' instead for the unified UI.

The UI provides:
  - List of all available tasks from Taskfile.yml
  - Click-to-run task execution with live output
  - Process-compose status view (if running)

Examples:
  xplat up                      # Unified UI (recommended)
  xplat ui                      # Legacy: Start on port 8760
  xplat ui -p 9000              # Start on port 9000
  xplat ui --no-browser         # Don't open browser`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use the unified app but with setup disabled (backward compat)
		cfg := web.DefaultAppConfig()
		cfg.Port = uiPort
		cfg.OpenBrowser = !uiNoBrowser
		cfg.ProcessComposePort = uiPCPort
		cfg.EnableSetup = false // Legacy mode: no setup wizard
		if uiTaskfile != "" {
			cfg.Taskfile = uiTaskfile
		}
		if uiDir != "" {
			cfg.WorkDir = uiDir
		}

		app, err := web.NewApp(cfg)
		if err != nil {
			return err
		}
		return app.Start(context.Background())
	},
}

func init() {
	UICmd.Flags().StringVarP(&uiPort, "port", "p", config.DefaultUIPort, "Port to listen on (default 8760)")
	UICmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "Don't open browser on start")
	UICmd.Flags().StringVarP(&uiTaskfile, "taskfile", "t", "", "Path to Taskfile.yml")
	UICmd.Flags().StringVarP(&uiDir, "dir", "d", "", "Working directory")
	UICmd.Flags().IntVar(&uiPCPort, "pc-port", config.DefaultProcessComposePort, "Process-compose API port (default 8761)")
}
