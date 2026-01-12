package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/syncgh"
)

// SyncGHCmd is the parent command for GitHub sync operations
var SyncGHCmd = &cobra.Command{
	Use:   "sync-gh",
	Short: "GitHub sync operations (no gh CLI required)",
	Long: `GitHub sync operations using go-github - no gh CLI needed.

Works identically on macOS, Linux, and Windows.
Designed to run as part of xplat service for continuous syncing.

Commands:
  state      Capture/display GitHub repo state (workflow runs, releases)
  check      Check for updates (one-time)
  poll       Poll for updates continuously
  webhook    Start webhook server
  tunnel     Forward smee.io webhooks to local server
  release    Get latest release tag for a repo

Environment:
  GITHUB_TOKEN    GitHub token for API (increases rate limit 60→5000/hour)

Examples:
  xplat sync-gh state joeblew999/xplat
  xplat sync-gh release nats-io/nats-server
  xplat sync-gh poll --interval=1h
  xplat sync-gh webhook --port=8080
  xplat sync-gh tunnel https://smee.io/xxx`,
}

var syncGHStateDir string
var syncGHShowOnly bool

var syncGHStateCmd = &cobra.Command{
	Use:   "state [owner/repo]",
	Short: "Capture or display GitHub repository state",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if syncGHShowOnly {
			state, err := syncgh.LoadState(syncGHStateDir)
			if err != nil {
				return fmt.Errorf("failed to load state: %w", err)
			}

			if state.SyncedAt.IsZero() {
				fmt.Println("No state found. Run 'xplat sync-gh state <repo>' to capture.")
				return nil
			}

			fmt.Print(syncgh.FormatState(state))
			return nil
		}

		// Capture state
		repo := ""
		if len(args) > 0 {
			repo = args[0]
		}
		if repo == "" {
			repo = os.Getenv("GITHUB_REPOSITORY")
		}
		if repo == "" {
			return fmt.Errorf("usage: xplat sync-gh state [owner/repo] [--show] [--dir=.github/state]")
		}

		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
		}

		log.Printf("Capturing state for %s...", repo)

		state, err := syncgh.CaptureState(parts[0], parts[1], os.Getenv("GITHUB_TOKEN"))
		if err != nil {
			return fmt.Errorf("failed to capture state: %w", err)
		}

		if err := syncgh.SaveState(state, syncGHStateDir); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}

		log.Printf("State captured:")
		log.Printf("  - Workflow runs: %d entries", len(state.WorkflowRuns))
		log.Printf("  - Pages builds: %d entries", len(state.PagesBuilds))
		if state.LatestRelease != nil {
			log.Printf("  - Latest release: %s", state.LatestRelease.TagName)
		} else {
			log.Printf("  - Latest release: none")
		}
		return nil
	},
}

var syncGHReleaseCmd = &cobra.Command{
	Use:   "release <owner/repo>",
	Short: "Get latest release tag for a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := args[0]
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
		}

		tag, err := syncgh.GetLatestRelease(parts[0], parts[1], os.Getenv("GITHUB_TOKEN"))
		if err != nil {
			return err
		}

		fmt.Println(tag)
		return nil
	},
}

var syncGHPollInterval string

var syncGHPollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll repositories for updates continuously",
	Long: `Poll GitHub repositories for updates continuously.

This is typically started automatically when xplat runs as a service.
Can also be run manually for testing.

Repos to poll are configured via xplat.yaml or command line.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		interval, err := time.ParseDuration(syncGHPollInterval)
		if err != nil {
			return fmt.Errorf("invalid interval: %w", err)
		}

		// Default repos to poll (can be overridden by config)
		repos := []syncgh.RepoConfig{
			// Add repos from config or args here
		}

		if len(repos) == 0 {
			log.Printf("No repos configured to poll. Use xplat.yaml to configure.")
			log.Printf("Running in demo mode - will just log poll cycles.")
		}

		poller := syncgh.NewPoller(interval, repos, os.Getenv("GITHUB_TOKEN"))
		poller.OnUpdate(func(subsystem, oldVersion, newVersion string) {
			log.Printf("Update detected: %s -> %s", subsystem, newVersion)
		})

		return poller.Start()
	},
}

var syncGHWebhookPort string

var syncGHWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Start webhook server",
	Run: func(cmd *cobra.Command, args []string) {
		syncgh.RunWebhook(syncGHWebhookPort)
	},
}

var syncGHTunnelCmd = &cobra.Command{
	Use:   "tunnel <smee-url|new> [target]",
	Short: "Forward smee.io webhooks to local server",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		smeeURL := args[0]
		target := "http://localhost:9090/webhook"
		if len(args) > 1 {
			target = args[1]
		}

		if smeeURL == "new" {
			smeeURL = syncgh.GenerateSmeeChannel()
			log.Printf("Created channel: %s", smeeURL)
		}

		syncgh.RunTunnel(smeeURL, target)
	},
}

var syncGHTunnelSetupEvents string

var syncGHTunnelSetupCmd = &cobra.Command{
	Use:   "tunnel-setup <owner/repo>",
	Short: "Create smee channel and configure GitHub webhook",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := args[0]

		smeeURL := syncgh.GenerateSmeeChannel()
		log.Printf("Created smee channel: %s", smeeURL)

		if err := syncgh.ConfigureGitHubWebhook(repo, smeeURL, syncGHTunnelSetupEvents); err != nil {
			return fmt.Errorf("failed to configure webhook: %w", err)
		}

		log.Printf("Webhook configured for %s", repo)
		log.Printf("")
		log.Printf("To start receiving webhooks:")
		log.Printf("  xplat sync-gh tunnel %s", smeeURL)
		return nil
	},
}

func init() {
	syncGHStateCmd.Flags().StringVar(&syncGHStateDir, "dir", ".github/state", "State directory")
	syncGHStateCmd.Flags().BoolVar(&syncGHShowOnly, "show", false, "Display current state without fetching")

	syncGHPollCmd.Flags().StringVar(&syncGHPollInterval, "interval", "1h", "Poll interval (e.g., 1h, 30m)")

	syncGHWebhookCmd.Flags().StringVar(&syncGHWebhookPort, "port", config.DefaultWebhookPort, "Webhook server port")

	syncGHTunnelSetupCmd.Flags().StringVar(&syncGHTunnelSetupEvents, "events", "push,release,workflow_run,page_build,deployment_status", "Webhook events")

	SyncGHCmd.AddCommand(syncGHStateCmd)
	SyncGHCmd.AddCommand(syncGHReleaseCmd)
	SyncGHCmd.AddCommand(syncGHPollCmd)
	SyncGHCmd.AddCommand(syncGHWebhookCmd)
	SyncGHCmd.AddCommand(syncGHTunnelCmd)
	SyncGHCmd.AddCommand(syncGHTunnelSetupCmd)
}
