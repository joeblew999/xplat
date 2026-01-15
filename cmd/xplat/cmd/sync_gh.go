package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/synccf"
	"github.com/joeblew999/xplat/internal/syncgh"
)

// SyncGHCmd is the parent command for GitHub sync operations
var SyncGHCmd = &cobra.Command{
	Use:   "sync-gh",
	Short: "GitHub sync operations (no gh CLI required)",
	Long: `GitHub sync operations using go-github - no gh CLI needed.

Works identically on macOS, Linux, and Windows.
Designed to run as part of xplat service for continuous syncing.

Quick Start (zero config real-time sync):
  xplat sync-gh relay
  # Then configure GitHub webhook to POST to the displayed tunnel URL

Commands:
  relay       Start webhook relay with CF tunnel (easiest setup)
  poll        Poll for updates continuously (no webhook needed)
  webhook     Start webhook server only
  sse-client  Connect to gosmee server for SSE relay
  state       Capture/display GitHub repo state
  release     Get latest release tag for a repo
  discover    Find repos from Taskfile.yml remote includes

Environment:
  GITHUB_TOKEN    GitHub token for API (increases rate limit 60→5000/hour)

Sync Methods:

  1. RELAY (recommended for development):
     xplat sync-gh relay
     - Starts webhook server + Cloudflare tunnel in one command
     - URL changes on restart (quick tunnel)

  2. POLLING (simplest, no webhooks):
     xplat sync-gh poll --interval=5m --invalidate
     - No setup required, just polls GitHub API
     - 5 minute delay between updates

  3. SSE CLIENT (for production with gosmee):
     xplat sync-gh sse-client https://webhook.example.com/abc123 --invalidate
     - Requires gosmee server with stable URL
     - Best for persistent connections with event replay

Examples:
  xplat sync-gh relay                    # Quick start with tunnel
  xplat sync-gh poll --invalidate        # Simple polling
  xplat sync-gh release nats-io/nats-server`,
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

var syncGHPollRepos string
var syncGHPollInvalidate bool

var syncGHPollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll repositories for updates continuously",
	Long: `Poll GitHub repositories for updates continuously.

Uses StatefulPoller to track commit hashes and only trigger on actual changes.
State is persisted to ~/.xplat/cache/syncgh-poll-state.json

If --repos is not specified, auto-discovers repos from Taskfile.yml remote includes.

Examples:
  # Auto-discover repos from Taskfile.yml
  xplat sync-gh poll

  # Poll a single repo
  xplat sync-gh poll --repos=joeblew999/xplat --interval=5m

  # Poll multiple repos
  xplat sync-gh poll --repos=joeblew999/xplat,go-task/task --interval=1h

  # Poll with Task cache invalidation
  xplat sync-gh poll --repos=joeblew999/xplat --invalidate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		interval, err := time.ParseDuration(syncGHPollInterval)
		if err != nil {
			return fmt.Errorf("invalid interval: %w", err)
		}

		workDir, _ := os.Getwd()

		// Parse repos from flag or auto-discover from Taskfile.yml
		var repos []syncgh.RepoConfig
		if syncGHPollRepos != "" {
			for _, r := range strings.Split(syncGHPollRepos, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					repos = append(repos, syncgh.RepoConfig{
						Subsystem: r,
						Branch:    "main",
					})
				}
			}
		} else {
			// Auto-discover from Taskfile.yml
			discovered, err := syncgh.DiscoverReposFromProject(workDir)
			if err != nil {
				log.Printf("Warning: failed to discover repos: %v", err)
			}
			repos = syncgh.DiscoverReposToConfigs(discovered)
		}

		if len(repos) == 0 {
			return fmt.Errorf("no repos found. Use --repos=owner/repo or add remote includes to Taskfile.yml")
		}

		log.Printf("Polling %d repos every %v", len(repos), interval)
		for _, r := range repos {
			log.Printf("  - %s", r.Subsystem)
		}

		// Use StatefulPoller for state persistence
		poller, err := syncgh.NewStatefulPoller(interval, repos, os.Getenv("GITHUB_TOKEN"))
		if err != nil {
			return fmt.Errorf("failed to create poller: %w", err)
		}

		// Wire up callback
		if syncGHPollInvalidate {
			log.Printf("Task cache invalidation enabled for: %s", workDir)
			poller.OnChange(syncgh.TaskCacheInvalidator(workDir))
		} else {
			poller.OnChange(func(repo, ref, oldHash, newHash string) {
				log.Printf("Change detected: %s@%s (%s -> %s)", repo, ref, oldHash, newHash)
			})
		}

		return poller.Start()
	},
}

var syncGHPollStateCmd = &cobra.Command{
	Use:   "poll-state",
	Short: "Show current poll state (tracked repos and commit hashes)",
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := syncgh.LoadPollState()
		if err != nil {
			return fmt.Errorf("failed to load poll state: %w", err)
		}

		if len(state.Repos) == 0 {
			fmt.Println("No repos tracked yet. Run 'xplat sync-gh poll' first.")
			return nil
		}

		fmt.Printf("Poll state (%s):\n", config.XplatCache()+"/syncgh-poll-state.json")
		fmt.Printf("Updated: %s\n\n", state.UpdatedAt.Format(time.RFC3339))

		for key, info := range state.Repos {
			fmt.Printf("  %s\n", key)
			fmt.Printf("    Commit:  %s\n", info.CommitHash)
			fmt.Printf("    Checked: %s\n", info.LastChecked.Format(time.RFC3339))
		}

		return nil
	},
}

var syncGHDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover GitHub repos from Taskfile.yml remote includes",
	Long: `Scan Taskfile.yml files in the current project for remote includes
and list the GitHub repos that would be watched.

This shows what repos the sync poller would auto-discover.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, _ := os.Getwd()

		repos, err := syncgh.DiscoverReposFromProject(workDir)
		if err != nil {
			return fmt.Errorf("failed to discover repos: %w", err)
		}

		if len(repos) == 0 {
			fmt.Println("No remote GitHub repos found in Taskfile.yml includes.")
			fmt.Println("Add remote includes like:")
			fmt.Println("  includes:")
			fmt.Println("    remote:")
			fmt.Println("      taskfile: https://raw.githubusercontent.com/owner/repo/main/Taskfile.yml")
			return nil
		}

		fmt.Printf("Discovered %d GitHub repo(s) from Taskfile.yml:\n\n", len(repos))
		for _, repo := range repos {
			fmt.Printf("  %s\n", repo)
		}

		fmt.Println()
		fmt.Println("To poll these repos:")
		fmt.Println("  xplat sync-gh poll --invalidate")

		return nil
	},
}

var syncGHWebhookPort string
var syncGHWebhookInvalidate bool

var syncGHWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Start webhook server",
	Long: `Start a webhook server to receive GitHub events.

When --invalidate is set, push events will trigger Task cache invalidation,
enabling real-time sync of remote taskfiles.

Examples:
  xplat sync-gh webhook --port=8763
  xplat sync-gh webhook --port=8763 --invalidate`,
	Run: func(cmd *cobra.Command, args []string) {
		if syncGHWebhookInvalidate {
			workDir, _ := os.Getwd()
			syncgh.RunWebhookWithInvalidation(syncGHWebhookPort, workDir)
		} else {
			syncgh.RunWebhook(syncGHWebhookPort)
		}
	},
}

var syncGHWebhookAddEvents string

var syncGHWebhookAddCmd = &cobra.Command{
	Use:   "webhook-add <owner/repo> <webhook-url>",
	Short: "Configure a GitHub repo to send webhooks to a URL",
	Long: `Add a webhook to a GitHub repository.

The webhook URL can be any publicly accessible URL, such as:
  - Cloudflare tunnel URL from 'xplat sync-cf tunnel'
  - Any server with a public IP

Requires GITHUB_TOKEN with repo admin permissions.

Example workflow for local development:
  1. Start webhook server:  xplat sync-gh webhook --port=8763
  2. Start CF tunnel:       xplat sync-cf tunnel --port=8763
  3. Copy the tunnel URL (e.g., https://xxx.trycloudflare.com)
  4. Add webhook:           xplat sync-gh webhook-add owner/repo https://xxx.trycloudflare.com/webhook`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := args[0]
		webhookURL := args[1]

		if err := syncgh.ConfigureGitHubWebhook(repo, webhookURL, syncGHWebhookAddEvents); err != nil {
			return fmt.Errorf("failed to configure webhook: %w", err)
		}

		return nil
	},
}

var syncGHWebhookListCmd = &cobra.Command{
	Use:   "webhook-list <owner/repo>",
	Short: "List webhooks configured on a GitHub repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return syncgh.ListWebhooks(args[0])
	},
}

var syncGHWebhookDeleteCmd = &cobra.Command{
	Use:   "webhook-delete <owner/repo> <hook-id>",
	Short: "Delete a webhook from a GitHub repo",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := args[0]
		var hookID int64
		if _, err := fmt.Sscanf(args[1], "%d", &hookID); err != nil {
			return fmt.Errorf("invalid hook ID: %s", args[1])
		}
		return syncgh.DeleteWebhook(repo, hookID)
	},
}

var syncGHSSETargetPort string
var syncGHSSESaveDir string
var syncGHSSEIgnoreEvents string
var syncGHSSEHealthPort int

// SSE Server flags
var syncGHServerPort string
var syncGHServerPublicURL string
var syncGHServerSecrets string

var syncGHRelayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Start webhook relay with Cloudflare tunnel (zero config real-time sync)",
	Long: `Start a complete webhook relay system with zero configuration.

This command:
  1. Installs cloudflared if needed
  2. Starts a cloudflared quick tunnel (gets public URL)
  3. Starts local webhook server with Task cache invalidation

The tunnel URL changes each time (quick tunnel), so you'll need to
update the GitHub webhook URL after each restart.

For stable URLs, use named tunnels (see 'xplat sync-cf' commands).

Usage:
  xplat sync-gh relay

Then configure GitHub webhook to POST to: <tunnel-url>/webhook
Push events will automatically invalidate your Task cache.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, _ := os.Getwd()

		// Start webhook server in background
		go func() {
			log.Printf("Starting local webhook handler on port %s with cache invalidation", syncGHWebhookPort)
			syncgh.RunWebhookWithInvalidation(syncGHWebhookPort, workDir)
		}()

		// Give server time to start
		time.Sleep(200 * time.Millisecond)

		// Setup signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Printf("Shutting down...")
			cancel()
		}()

		// Start cloudflared tunnel
		log.Printf("Starting Cloudflare quick tunnel...")
		log.Printf("")
		log.Printf("Once the tunnel URL appears, configure your GitHub webhook to POST to:")
		log.Printf("  <tunnel-url>/webhook")
		log.Printf("")

		portInt := 8763
		if _, err := fmt.Sscanf(syncGHWebhookPort, "%d", &portInt); err != nil {
			portInt = 8763
		}

		return synccf.RunTunnel(ctx, portInt)
	},
}

var syncGHSSEClientCmd = &cobra.Command{
	Use:   "sse-client <server-url>",
	Short: "Connect to a gosmee server and forward events to local webhook handler",
	Long: `Connect to a gosmee server via SSE and forward GitHub webhook events
to the local webhook handler for Task cache invalidation.

This enables real-time sync when combined with:
  1. A gosmee server running behind a Cloudflare tunnel (or xplat sync-gh server)
  2. GitHub webhooks pointing to the server URL

Architecture:
  GitHub → SSE server (stable URL) → SSE → this client → local webhook → cache invalidation

Example usage:
  # Start local webhook handler + SSE client (all-in-one)
  xplat sync-gh sse-client https://webhook.example.com/abc123 --invalidate

  # Just the SSE client (if webhook server is running separately)
  xplat sync-gh sse-client https://webhook.example.com/abc123 --port=8763

  # Save payloads for debugging/replay
  xplat sync-gh sse-client https://webhook.example.com/abc123 --save-dir=./webhooks

  # Ignore certain event types
  xplat sync-gh sse-client https://webhook.example.com/abc123 --ignore-event=ping,status

  # Enable health endpoint for K8s probes
  xplat sync-gh sse-client https://webhook.example.com/abc123 --health-port=8080`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := args[0]

		// Parse ignore events
		var ignoreEvents []string
		if syncGHSSEIgnoreEvents != "" {
			for _, e := range strings.Split(syncGHSSEIgnoreEvents, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					ignoreEvents = append(ignoreEvents, e)
				}
			}
		}

		if syncGHWebhookInvalidate {
			// All-in-one: local webhook handler + SSE client + cache invalidation
			workDir, _ := os.Getwd()
			return syncgh.RunSSEClientWithInvalidation(serverURL, workDir, syncGHSSETargetPort, syncGHSSESaveDir, ignoreEvents, syncGHSSEHealthPort)
		}

		// Use full config for advanced options
		targetURL := fmt.Sprintf("http://localhost:%s/webhook", syncGHSSETargetPort)
		return syncgh.RunSSEClientWithOptions(context.Background(), syncgh.SSEClientConfig{
			ServerURL:    serverURL,
			TargetURL:    targetURL,
			SaveDir:      syncGHSSESaveDir,
			IgnoreEvents: ignoreEvents,
			HealthPort:   syncGHSSEHealthPort,
		})
	},
}

// Replay command flags
var syncGHReplayListHooks bool
var syncGHReplayListDeliveries bool
var syncGHReplaySince string
var syncGHReplaySaveDir string
var syncGHReplayIgnoreEvents string
var syncGHReplayContinuous bool

var syncGHReplayCmd = &cobra.Command{
	Use:   "replay <owner/repo> [hook-id] [target-url]",
	Short: "Replay webhook deliveries from GitHub API",
	Long: `Fetch and replay webhook deliveries from GitHub API.

This is useful when:
  - You missed webhooks while your relay was down
  - Testing webhook handlers with real payloads
  - Debugging webhook processing issues

Requires GITHUB_TOKEN with repo/org admin permissions.

Examples:
  # List webhooks on a repo
  xplat sync-gh replay owner/repo --list-hooks

  # List webhooks on an org
  xplat sync-gh replay myorg --list-hooks

  # List recent deliveries for a hook
  xplat sync-gh replay owner/repo 12345 --list-deliveries

  # Replay all deliveries since a timestamp
  xplat sync-gh replay owner/repo 12345 http://localhost:8763/webhook --since=2024-01-01T00:00:00

  # Continuous mode - keep watching for new deliveries
  xplat sync-gh replay owner/repo 12345 http://localhost:8763/webhook --continuous

  # Save payloads while replaying
  xplat sync-gh replay owner/repo 12345 http://localhost:8763/webhook --save-dir=./webhooks`,
	Args: cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse owner/repo
		orgRepo := args[0]
		var owner, repo string
		if strings.Contains(orgRepo, "/") {
			parts := strings.Split(orgRepo, "/")
			owner = parts[0]
			repo = parts[1]
		} else {
			owner = orgRepo
		}

		token := os.Getenv("GITHUB_TOKEN")

		// List hooks
		if syncGHReplayListHooks {
			return syncgh.RunReplayListHooks(owner, repo, token)
		}

		// Need hook ID for further operations
		if len(args) < 2 {
			return fmt.Errorf("hook ID is required. Use --list-hooks to find hook IDs")
		}

		var hookID int64
		if _, err := fmt.Sscanf(args[1], "%d", &hookID); err != nil {
			return fmt.Errorf("invalid hook ID: %s", args[1])
		}

		// List deliveries
		if syncGHReplayListDeliveries {
			return syncgh.RunReplayListDeliveries(owner, repo, hookID, token)
		}

		// Need target URL for replay
		if len(args) < 3 {
			return fmt.Errorf("target URL is required for replay (e.g., http://localhost:8763/webhook)")
		}

		targetURL := args[2]

		// Parse since time
		var sinceTime time.Time
		if syncGHReplaySince != "" {
			var err error
			sinceTime, err = time.Parse("2006-01-02T15:04:05", syncGHReplaySince)
			if err != nil {
				return fmt.Errorf("invalid --since format (use 2006-01-02T15:04:05): %w", err)
			}
		}

		// Parse ignore events
		var ignoreEvents []string
		if syncGHReplayIgnoreEvents != "" {
			for _, e := range strings.Split(syncGHReplayIgnoreEvents, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					ignoreEvents = append(ignoreEvents, e)
				}
			}
		}

		return syncgh.RunReplay(syncgh.ReplayConfig{
			Owner:        owner,
			Repo:         repo,
			HookID:       hookID,
			TargetURL:    targetURL,
			Since:        sinceTime,
			SaveDir:      syncGHReplaySaveDir,
			IgnoreEvents: ignoreEvents,
			Continuous:   syncGHReplayContinuous,
			Token:        token,
		})
	},
}

var syncGHServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Start a gosmee-compatible SSE server for webhook relay",
	Long: `Start a gosmee-compatible SSE server that receives GitHub webhooks
and broadcasts them to connected SSE clients.

This eliminates the need for external services like smee.io or gosmee.net.
Run your own webhook relay server behind Cloudflare tunnel for full control.

Endpoints:
  GET  /health          Health check
  GET  /new             Generate a new channel URL
  GET  /events/{channel} SSE event stream
  POST /{channel}       Receive webhooks
  GET  /{channel}       Channel info page

Example usage:
  # Start server on default port
  xplat sync-gh server

  # With custom port and public URL
  xplat sync-gh server --port=3333 --public-url=https://webhook.example.com

  # With webhook signature validation
  xplat sync-gh server --secrets=secret1,secret2

Full relay setup:
  1. Start server:  xplat sync-gh server --port=3333
  2. Start tunnel:  xplat sync-cf tunnel --port=3333
  3. Configure GitHub webhook to: <tunnel-url>/<channel>
  4. Connect client: xplat sync-gh sse-client <tunnel-url>/<channel> --invalidate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var secrets []string
		if syncGHServerSecrets != "" {
			for _, s := range strings.Split(syncGHServerSecrets, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					secrets = append(secrets, s)
				}
			}
		}

		return syncgh.RunSSEServer(syncGHServerPort, syncGHServerPublicURL, secrets)
	},
}

func init() {
	syncGHStateCmd.Flags().StringVar(&syncGHStateDir, "dir", ".github/state", "State directory")
	syncGHStateCmd.Flags().BoolVar(&syncGHShowOnly, "show", false, "Display current state without fetching")

	syncGHPollCmd.Flags().StringVar(&syncGHPollInterval, "interval", config.DefaultSyncInterval, "Poll interval (e.g., 5m, 1h)")
	syncGHPollCmd.Flags().StringVar(&syncGHPollRepos, "repos", "", "Repos to poll (comma-separated: owner/repo,owner2/repo2)")
	syncGHPollCmd.Flags().BoolVar(&syncGHPollInvalidate, "invalidate", false, "Invalidate Task cache on change")

	syncGHWebhookCmd.Flags().StringVar(&syncGHWebhookPort, "port", config.DefaultWebhookPort, "Webhook server port")
	syncGHWebhookCmd.Flags().BoolVar(&syncGHWebhookInvalidate, "invalidate", false, "Invalidate Task cache on push events")

	syncGHWebhookAddCmd.Flags().StringVar(&syncGHWebhookAddEvents, "events", "push,release,workflow_run,page_build,deployment_status", "Webhook events")

	syncGHSSEClientCmd.Flags().StringVar(&syncGHSSETargetPort, "port", config.DefaultWebhookPort, "Local webhook server port")
	syncGHSSEClientCmd.Flags().BoolVar(&syncGHWebhookInvalidate, "invalidate", false, "Start local webhook server with cache invalidation")
	syncGHSSEClientCmd.Flags().StringVar(&syncGHSSESaveDir, "save-dir", "", "Save webhook payloads to disk for debugging/replay")
	syncGHSSEClientCmd.Flags().StringVar(&syncGHSSEIgnoreEvents, "ignore-event", "", "Comma-separated event types to ignore (e.g., ping,status)")
	syncGHSSEClientCmd.Flags().IntVar(&syncGHSSEHealthPort, "health-port", 0, "Port for health endpoint (0 = disabled)")

	syncGHServerCmd.Flags().StringVar(&syncGHServerPort, "port", "3333", "Server port")
	syncGHServerCmd.Flags().StringVar(&syncGHServerPublicURL, "public-url", "", "Public URL for webhook configuration (optional)")
	syncGHServerCmd.Flags().StringVar(&syncGHServerSecrets, "secrets", "", "Comma-separated webhook secrets for signature validation")

	syncGHReplayCmd.Flags().BoolVar(&syncGHReplayListHooks, "list-hooks", false, "List webhooks on the repo/org")
	syncGHReplayCmd.Flags().BoolVar(&syncGHReplayListDeliveries, "list-deliveries", false, "List recent deliveries for a hook")
	syncGHReplayCmd.Flags().StringVar(&syncGHReplaySince, "since", "", "Replay deliveries since this time (format: 2006-01-02T15:04:05)")
	syncGHReplayCmd.Flags().StringVar(&syncGHReplaySaveDir, "save-dir", "", "Save payloads to disk for debugging/replay")
	syncGHReplayCmd.Flags().StringVar(&syncGHReplayIgnoreEvents, "ignore-event", "", "Comma-separated event types to ignore")
	syncGHReplayCmd.Flags().BoolVar(&syncGHReplayContinuous, "continuous", false, "Keep watching for new deliveries")

	syncGHRelayCmd.Flags().StringVar(&syncGHWebhookPort, "port", config.DefaultWebhookPort, "Local webhook server port")

	SyncGHCmd.AddCommand(syncGHDiscoverCmd)
	SyncGHCmd.AddCommand(syncGHPollCmd)
	SyncGHCmd.AddCommand(syncGHPollStateCmd)
	SyncGHCmd.AddCommand(syncGHRelayCmd)
	SyncGHCmd.AddCommand(syncGHReleaseCmd)
	SyncGHCmd.AddCommand(syncGHReplayCmd)
	SyncGHCmd.AddCommand(syncGHServerCmd)
	SyncGHCmd.AddCommand(syncGHSSEClientCmd)
	SyncGHCmd.AddCommand(syncGHStateCmd)
	SyncGHCmd.AddCommand(syncGHWebhookCmd)
	SyncGHCmd.AddCommand(syncGHWebhookAddCmd)
	SyncGHCmd.AddCommand(syncGHWebhookDeleteCmd)
	SyncGHCmd.AddCommand(syncGHWebhookListCmd)
}
