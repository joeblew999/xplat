package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	web "github.com/joeblew999/xplat/internal/webui"
)

var (
	upPort        string
	upNoBrowser   bool
	upTaskfile    string
	upDir         string
	upPCPort      int
	upNoTasks     bool
	upNoProcesses bool
	upNoSetup     bool
)

// UpCmd starts the unified xplat web UI.
var UpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the unified xplat web UI",
	Long: `Start the unified xplat web interface with all features enabled.

This is the primary way to run xplat's web UI. It provides:
  - Dashboard: Overview of your project
  - Tasks: Run Taskfile tasks with live output
  - Processes: Monitor process-compose processes
  - Setup: Configure environment and services

The UI is driven by your project's configuration (Taskfile.yml, process-compose.yaml).

Examples:
  xplat up                     # Start with all features on port 8760
  xplat up -p 9000             # Start on port 9000
  xplat up --no-browser        # Don't open browser (for service mode)
  xplat up --no-setup          # Disable setup wizard
  xplat up -d /path/to/project # Use specific project directory`,
	RunE: runUp,
}

func init() {
	UpCmd.Flags().StringVarP(&upPort, "port", "p", config.DefaultUIPort, "Port to listen on")
	UpCmd.Flags().BoolVar(&upNoBrowser, "no-browser", false, "Don't open browser on start")
	UpCmd.Flags().StringVarP(&upTaskfile, "taskfile", "t", "", "Path to Taskfile.yml")
	UpCmd.Flags().StringVarP(&upDir, "dir", "d", "", "Working directory")
	UpCmd.Flags().IntVar(&upPCPort, "pc-port", config.DefaultProcessComposePort, "Process-compose API port")
	UpCmd.Flags().BoolVar(&upNoTasks, "no-tasks", false, "Disable task UI")
	UpCmd.Flags().BoolVar(&upNoProcesses, "no-processes", false, "Disable process view")
	UpCmd.Flags().BoolVar(&upNoSetup, "no-setup", false, "Disable setup wizard")
}

func runUp(cmd *cobra.Command, args []string) error {
	cfg := web.DefaultAppConfig()
	cfg.Port = upPort
	cfg.OpenBrowser = !upNoBrowser
	cfg.ProcessComposePort = upPCPort
	cfg.EnableTasks = !upNoTasks
	cfg.EnableProcesses = !upNoProcesses
	cfg.EnableSetup = !upNoSetup

	if upTaskfile != "" {
		cfg.Taskfile = upTaskfile
	}
	if upDir != "" {
		cfg.WorkDir = upDir
	}

	app, err := web.NewApp(cfg)
	if err != nil {
		return err
	}

	return app.Start(context.Background())
}
