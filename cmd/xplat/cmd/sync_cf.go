package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joeblew999/xplat/internal/env"
	"github.com/joeblew999/xplat/internal/synccf"
	"github.com/spf13/cobra"
)

// getReceiverPort returns the port for the receiver server.
// Priority: CLI flag > .env file > default
func getReceiverPort(flagValue string) string {
	// CLI flag takes precedence
	if flagValue != "" && flagValue != "9091" {
		return flagValue
	}

	// Try to load from .env
	cfg, err := env.LoadEnv()
	if err == nil && cfg != nil {
		port := cfg.Get(env.KeyCloudflareReceiverPort)
		if port != "" && port != "9091" {
			return port
		}
	}

	// Return flag value (which may be the default)
	return flagValue
}

// SyncCFCmd is the parent command for Cloudflare sync operations
var SyncCFCmd = &cobra.Command{
	Use:   "sync-cf",
	Short: "Cloudflare sync operations (no wrangler CLI required)",
	Long: `Cloudflare sync operations - no wrangler CLI needed.

Works identically on macOS, Linux, and Windows.
Designed to run as part of xplat service for continuous syncing.

Commands:
  receive        Receive events from CF Worker (round-trip validation)
  receive-state  Show current receive state
  auth           Set up R2 credentials interactively
  tunnel         Start cloudflared tunnel (quick or named)
  tunnel-login   Authenticate cloudflared with Cloudflare
  tunnel-list    List existing named tunnels
  tunnel-create  Create a new named tunnel
  tunnel-delete  Delete a named tunnel
  tunnel-route   Add DNS route for a tunnel
  poll           Poll CF audit logs continuously
  webhook        Start CF webhook server
  check          Check if cloudflared is installed
  install        Install cloudflared
  worker         Deploy sync-cf worker to Cloudflare edge

Environment:
  CF_ACCOUNT_ID       Cloudflare account ID
  CF_API_TOKEN        Cloudflare API token
  R2_ACCESS_KEY       R2 API access key
  R2_SECRET_KEY       R2 API secret key

Round-Trip Validation (recommended):
  1. Deploy CF Worker:  xplat sync-cf worker deploy
  2. Start receiver:    xplat sync-cf receive --port=9091
  3. Start tunnel:      xplat sync-cf tunnel 9091
  4. Configure SYNC_ENDPOINT on Worker to tunnel URL

Quick Tunnel (random URL, no account needed):
  xplat sync-cf tunnel 8080

Named Tunnel (stable URL, requires CF account + domain):
  1. xplat sync-cf tunnel-login           # One-time: authenticate
  2. xplat sync-cf tunnel-create webhook  # One-time: create tunnel
  3. xplat sync-cf tunnel-route webhook webhook.yourdomain.com
  4. xplat sync-cf tunnel --name=webhook  # Run with stable URL

Examples:
  xplat sync-cf receive --port=9091
  xplat sync-cf auth
  xplat sync-cf check
  xplat sync-cf tunnel 8080
  xplat sync-cf tunnel --name=webhook --port=8080
  xplat sync-cf poll --interval=1m
  xplat sync-cf webhook --port=9090
  xplat sync-cf worker deploy`,
}

var syncCFTunnelName string
var syncCFTunnelPort string
var syncCFReceivePort string
var syncCFReceiveInvalidate bool

var syncCFReceiveCmd = &cobra.Command{
	Use:   "receive",
	Short: "Receive events from CF Worker (round-trip validation)",
	Long: `Start an HTTP server to receive events forwarded by the CF Worker.

This completes the round-trip validation flow:
  1. CF Worker receives events from Cloudflare services
  2. Worker normalizes and forwards to SYNC_ENDPOINT (this receiver)
  3. Receiver logs events and can trigger cache invalidation

Architecture:
  Cloudflare → CF Worker → Tunnel → this receiver → callbacks

The receiver supports these event types:
  - pages_deploy: Pages deploy hooks (triggers cache invalidation with --invalidate)
  - alert: Notification webhooks
  - logpush: Logpush HTTP destination batches

Examples:
  # Start receiver on default port
  xplat sync-cf receive

  # Start receiver with Task cache invalidation
  xplat sync-cf receive --invalidate

  # Start receiver with custom port
  xplat sync-cf receive --port=9091

  # Start receiver + tunnel together
  xplat sync-cf receive --port=9091 --invalidate &
  xplat sync-cf tunnel 9091`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get port from flag or .env
		port := getReceiverPort(syncCFReceivePort)

		callbacks := synccf.ReceiveCallbacks{
			OnAny: synccf.DefaultLogCallback(),
		}

		if syncCFReceiveInvalidate {
			workDir, _ := os.Getwd()
			log.Printf("Task cache invalidation enabled for: %s", workDir)
			callbacks.OnPagesDeploy = synccf.TaskCacheInvalidator(workDir)
		}

		return synccf.RunReceiveServer(port, callbacks)
	},
}

var syncCFReceiveStateCmd = &cobra.Command{
	Use:   "receive-state",
	Short: "Show current receive state (processed events)",
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := synccf.LoadReceiveState()
		if err != nil {
			return err
		}

		if state.UpdatedAt.IsZero() {
			log.Printf("No events received yet. Run 'xplat sync-cf receive' first.")
			return nil
		}

		log.Printf("Receive state:")
		log.Printf("  Updated: %s", state.UpdatedAt.Format(time.RFC3339))
		log.Printf("  Last event: %s", state.LastEventTime.Format(time.RFC3339))
		log.Printf("  Events processed: %d", len(state.ProcessedEvents))

		// Show recent events
		if len(state.ProcessedEvents) > 0 {
			log.Printf("")
			log.Printf("Recent events:")
			count := 0
			for key, event := range state.ProcessedEvents {
				if count >= 10 {
					log.Printf("  ... and %d more", len(state.ProcessedEvents)-10)
					break
				}
				log.Printf("  [%s] %s on %s (at %s)",
					event.Type, event.Action, event.Resource,
					event.ProcessedAt.Format(time.RFC3339))
				_ = key
				count++
			}
		}

		return nil
	},
}

var syncCFTunnelCmd = &cobra.Command{
	Use:   "tunnel [port]",
	Short: "Start cloudflared tunnel (quick or named)",
	Long: `Start a cloudflared tunnel to expose a local port to the internet.

Quick Tunnel (default):
  Random URL like https://xxx.trycloudflare.com
  No account needed, URL changes on each restart

Named Tunnel (--name flag):
  Stable URL tied to your Cloudflare domain
  Requires prior setup: tunnel-login, tunnel-create, tunnel-route

Examples:
  xplat sync-cf tunnel 8080                      # Quick tunnel
  xplat sync-cf tunnel --port=8080               # Quick tunnel with flag
  xplat sync-cf tunnel --name=webhook --port=8080  # Named tunnel`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get port from flag > args > .env > default
		port := 9091 // default for sync receiver
		if syncCFTunnelPort != "" {
			if p, err := strconv.Atoi(syncCFTunnelPort); err == nil && p > 0 {
				port = p
			}
		} else if len(args) > 0 {
			if p, err := strconv.Atoi(args[0]); err == nil && p > 0 {
				port = p
			}
		} else {
			// Try to get from .env
			receiverPort := getReceiverPort("9091")
			if p, err := strconv.Atoi(receiverPort); err == nil && p > 0 {
				port = p
			}
		}

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		if syncCFTunnelName != "" {
			return synccf.RunNamedTunnel(ctx, syncCFTunnelName, port)
		}
		return synccf.RunTunnel(ctx, port)
	},
}

var syncCFTunnelLoginCmd = &cobra.Command{
	Use:   "tunnel-login",
	Short: "Authenticate cloudflared with Cloudflare",
	Long: `Authenticate cloudflared with your Cloudflare account.

This opens a browser for OAuth authentication and stores
credentials at ~/.cloudflared/cert.pem.

Required before creating named tunnels.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.LoginCloudflared()
	},
}

var syncCFTunnelListCmd = &cobra.Command{
	Use:   "tunnel-list",
	Short: "List existing named tunnels",
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.ListTunnels()
	},
}

var syncCFTunnelCreateCmd = &cobra.Command{
	Use:   "tunnel-create <name>",
	Short: "Create a new named tunnel",
	Long: `Create a new named tunnel.

After creation, add a DNS route to make it accessible:
  xplat sync-cf tunnel-route <name> <hostname>

Example:
  xplat sync-cf tunnel-create webhook
  xplat sync-cf tunnel-route webhook webhook.yourdomain.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.CreateTunnel(args[0])
	},
}

var syncCFTunnelDeleteCmd = &cobra.Command{
	Use:   "tunnel-delete <name>",
	Short: "Delete a named tunnel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.DeleteTunnel(args[0])
	},
}

var syncCFTunnelRouteCmd = &cobra.Command{
	Use:   "tunnel-route <tunnel-name> <hostname>",
	Short: "Add DNS route for a tunnel",
	Long: `Create a DNS CNAME record pointing to a tunnel.

The hostname must be on a domain managed by Cloudflare.

Example:
  xplat sync-cf tunnel-route webhook webhook.yourdomain.com

This creates: webhook.yourdomain.com -> tunnel`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.RouteTunnelDNS(args[0], args[1])
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

var syncCFAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Set up Cloudflare credentials interactively",
	Long: `Interactive authentication setup for Cloudflare.

Guides you through setting up all Cloudflare credentials:
  1. Account ID (required)
  2. API Token (optional - for Workers, Pages, DNS)
  3. R2 credentials (optional - for object storage)

Opens the relevant Cloudflare dashboard pages in your browser and
saves credentials to your .env file.

Saved credentials:
  CF_ACCOUNT_ID    - Your Cloudflare account ID
  CF_API_TOKEN     - Cloudflare API token (optional)
  R2_ACCESS_KEY    - R2 API access key
  R2_SECRET_KEY    - R2 API secret key`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return synccf.RunAuth(os.Stdout)
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
	// Receive flags
	syncCFReceiveCmd.Flags().StringVar(&syncCFReceivePort, "port", "9091", "Receive server port")
	syncCFReceiveCmd.Flags().BoolVar(&syncCFReceiveInvalidate, "invalidate", false, "Invalidate Task cache on Pages deploy events")

	syncCFPollCmd.Flags().StringVar(&syncCFPollInterval, "interval", "1m", "Poll interval")
	syncCFWebhookCmd.Flags().StringVar(&syncCFWebhookPort, "port", "9090", "Webhook server port")

	// Tunnel flags
	syncCFTunnelCmd.Flags().StringVar(&syncCFTunnelName, "name", "", "Named tunnel name (for stable URL)")
	syncCFTunnelCmd.Flags().StringVar(&syncCFTunnelPort, "port", "", "Local port to expose")

	SyncCFCmd.AddCommand(syncCFAuthCmd)
	SyncCFCmd.AddCommand(syncCFCheckCmd)
	SyncCFCmd.AddCommand(syncCFInstallCmd)
	SyncCFCmd.AddCommand(syncCFPollCmd)
	SyncCFCmd.AddCommand(syncCFReceiveCmd)
	SyncCFCmd.AddCommand(syncCFReceiveStateCmd)
	SyncCFCmd.AddCommand(syncCFTunnelCmd)
	SyncCFCmd.AddCommand(syncCFTunnelCreateCmd)
	SyncCFCmd.AddCommand(syncCFTunnelDeleteCmd)
	SyncCFCmd.AddCommand(syncCFTunnelListCmd)
	SyncCFCmd.AddCommand(syncCFTunnelLoginCmd)
	SyncCFCmd.AddCommand(syncCFTunnelRouteCmd)
	SyncCFCmd.AddCommand(syncCFWebhookCmd)

	// Worker subcommands
	syncCFWorkerCmd.AddCommand(syncCFWorkerBuildCmd)
	syncCFWorkerCmd.AddCommand(syncCFWorkerDeployCmd)
	syncCFWorkerCmd.AddCommand(syncCFWorkerRunCmd)
	SyncCFCmd.AddCommand(syncCFWorkerCmd)
}
