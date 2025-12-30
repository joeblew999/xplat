package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joeblew999/xplat/internal/synccf"
	"github.com/spf13/cobra"
)

// SyncCFCmd is the parent command for Cloudflare sync operations
var SyncCFCmd = &cobra.Command{
	Use:   "sync-cf",
	Short: "Cloudflare sync operations (no wrangler CLI required)",
	Long: `Cloudflare sync operations - no wrangler CLI needed.

Works identically on macOS, Linux, and Windows.
Designed to run as part of xplat service for continuous syncing.

Commands:
  tunnel     Start cloudflared quick tunnel
  poll       Poll CF audit logs continuously
  webhook    Start CF webhook server
  check      Check if cloudflared is installed
  install    Install cloudflared
  worker     Deploy sync-cf worker to Cloudflare edge

Environment:
  CF_ACCOUNT_ID       Cloudflare account ID
  CF_API_TOKEN        Cloudflare API token
  CF_WEBHOOK_SECRET   Cloudflare webhook secret

Examples:
  xplat sync-cf check
  xplat sync-cf tunnel 8080
  xplat sync-cf poll --interval=1m
  xplat sync-cf webhook --port=9090
  xplat sync-cf worker deploy`,
}

var syncCFTunnelCmd = &cobra.Command{
	Use:   "tunnel [port]",
	Short: "Start cloudflared quick tunnel",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port := 9090
		if len(args) > 0 {
			if p, err := strconv.Atoi(args[0]); err == nil && p > 0 {
				port = p
			}
		}

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		return synccf.RunTunnel(ctx, port)
	},
}

var syncCFPollInterval string

var syncCFPollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll CF audit logs continuously",
	RunE: func(cmd *cobra.Command, args []string) error {
		interval, err := time.ParseDuration(syncCFPollInterval)
		if err != nil {
			interval = time.Minute
		}

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		return synccf.RunPoll(ctx,
			os.Getenv("CF_ACCOUNT_ID"),
			os.Getenv("CF_API_TOKEN"),
			interval,
		)
	},
}

var syncCFWebhookPort string

var syncCFWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Start CF webhook server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.RunWebhookServer(syncCFWebhookPort,
			os.Getenv("CF_ACCOUNT_ID"),
			os.Getenv("CF_API_TOKEN"),
			os.Getenv("CF_WEBHOOK_SECRET"),
		)
	},
}

var syncCFCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if cloudflared is installed",
	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := synccf.GetCloudflaredInfo()
		if err != nil {
			log.Printf("cloudflared not installed")
			log.Printf("   Run: xplat sync-cf install")
			os.Exit(1)
		}
		log.Printf("cloudflared is installed: %s", info.Version)
		log.Printf("   Path: %s", info.Path)
		return nil
	},
}

var syncCFInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install cloudflared",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := synccf.InstallCloudflared(); err != nil {
			return err
		}
		log.Printf("cloudflared installed successfully")
		return nil
	},
}

var syncCFWorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Manage sync-cf Cloudflare Worker",
	Long: `Manage the sync-cf Cloudflare Worker.

The worker runs on Cloudflare's edge and aggregates events from
Pages deploys, Notifications, and Logpush, forwarding them to
your xplat sync service.

Commands:
  xplat sync-cf worker build     Build WASM binary
  xplat sync-cf worker run       Run local dev server
  xplat sync-cf worker deploy    Deploy to Cloudflare

The worker source is in workers/sync-cf/`,
}

var syncCFWorkerBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build worker WASM binary",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Building sync-cf worker...")
		log.Printf("  cd workers/sync-cf && xplat task build")
		log.Printf("")
		log.Printf("Requires TinyGo for WASM compilation (fits free tier)")
	},
}

var syncCFWorkerDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy worker to Cloudflare",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Deploying sync-cf worker...")
		log.Printf("  cd workers/sync-cf && xplat task deploy")
		log.Printf("")
		log.Printf("Requires wrangler CLI and CLOUDFLARE_API_TOKEN")
	},
}

var syncCFWorkerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run local development server",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Running sync-cf worker locally...")
		log.Printf("  cd workers/sync-cf && xplat task run")
		log.Printf("")
		log.Printf("Access at http://localhost:8787")
	},
}

func init() {
	syncCFPollCmd.Flags().StringVar(&syncCFPollInterval, "interval", "1m", "Poll interval")
	syncCFWebhookCmd.Flags().StringVar(&syncCFWebhookPort, "port", "9090", "Webhook server port")

	SyncCFCmd.AddCommand(syncCFTunnelCmd)
	SyncCFCmd.AddCommand(syncCFPollCmd)
	SyncCFCmd.AddCommand(syncCFWebhookCmd)
	SyncCFCmd.AddCommand(syncCFCheckCmd)
	SyncCFCmd.AddCommand(syncCFInstallCmd)

	// Worker subcommands
	syncCFWorkerCmd.AddCommand(syncCFWorkerBuildCmd)
	syncCFWorkerCmd.AddCommand(syncCFWorkerDeployCmd)
	syncCFWorkerCmd.AddCommand(syncCFWorkerRunCmd)
	SyncCFCmd.AddCommand(syncCFWorkerCmd)
}
