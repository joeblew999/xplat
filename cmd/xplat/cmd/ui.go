package cmd

import (
	"context"

	"github.com/spf13/cobra"

	web "github.com/joeblew999/xplat/internal/webui"
)

var uiPort string
var uiNoBrowser bool
var uiTaskfile string
var uiDir string
var uiPCPort int

// UICmd starts the Task UI web interface
var UICmd = &cobra.Command{
	Use:   "ui",
	Short: "Start Task UI web interface",
	Long: `Start a web-based UI for running Taskfile tasks.

The UI provides:
  - List of all available tasks from Taskfile.yml
  - Click-to-run task execution with live output
  - Process-compose status view (if running)

Examples:
  xplat ui                    # Start on port 3000, open browser
  xplat ui -p 8000            # Start on port 8000
  xplat ui --no-browser       # Don't open browser (for service mode)
  xplat ui -d /path/to/project  # Use specific project directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := web.DefaultViaConfig()
		cfg.Port = uiPort
		cfg.OpenBrowser = !uiNoBrowser
		cfg.ProcessComposePort = uiPCPort
		if uiTaskfile != "" {
			cfg.Taskfile = uiTaskfile
		}
		if uiDir != "" {
			cfg.WorkDir = uiDir
		}
		return web.StartVia(context.Background(), cfg)
	},
}

func init() {
	UICmd.Flags().StringVarP(&uiPort, "port", "p", "3000", "Port to listen on")
	UICmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "Don't open browser on start")
	UICmd.Flags().StringVarP(&uiTaskfile, "taskfile", "t", "", "Path to Taskfile.yml")
	UICmd.Flags().StringVarP(&uiDir, "dir", "d", "", "Working directory")
	UICmd.Flags().IntVar(&uiPCPort, "pc-port", 8080, "Process-compose API port")
}
