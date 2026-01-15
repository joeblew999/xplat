// Package syncgh provides GitHub synchronization utilities for xplat.
//
// # Components
//
//   - Poller: Poll GitHub repos periodically for changes (commit hashes, tags)
//   - StatefulPoller: Poller with state persistence - only triggers on actual changes
//   - PollState: Tracks commit hashes between polls (~/.xplat/cache/syncgh-poll-state.json)
//   - DiscoverReposFromProject: Auto-discover GitHub repos from Taskfile.yml remote includes
//   - TaskCacheInvalidator: Callback to invalidate Task remote taskfile cache on change
//   - Webhook: HTTP server to receive GitHub webhook events
//   - SSEServer: gosmee-compatible SSE server for webhook relay
//   - SSEClient: SSE client for receiving webhooks from gosmee/SSE server
//   - Replayer: Fetch and replay past webhook deliveries from GitHub API
//   - Tunnel: smee.io forwarding for local webhook development
//   - State: Snapshot and persist GitHub repo state (workflow runs, releases)
//
// # Poller Usage (Basic - No State)
//
// The basic Poller checks GitHub repos at a configurable interval.
// It does NOT track state - caller must compare oldVersion (always empty):
//
//	repos := []syncgh.RepoConfig{
//	    {Subsystem: "owner/repo", Branch: "main"},
//	    {Subsystem: "owner/repo2", UseTag: true, Tag: "v1.0.0"},
//	}
//
//	poller := syncgh.NewPoller(1*time.Hour, repos, os.Getenv("GITHUB_TOKEN"))
//	poller.OnUpdate(func(subsystem, oldVersion, newVersion string) {
//	    // NOTE: oldVersion is ALWAYS empty - you must track state yourself
//	})
//	poller.StartAsync()
//
// # StatefulPoller Usage (Recommended - With State)
//
// StatefulPoller tracks commit hashes between polls and only triggers
// callbacks when changes are detected:
//
//	poller, err := syncgh.NewStatefulPoller(1*time.Hour, repos, token)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use built-in Task cache invalidator
//	poller.OnChange(syncgh.TaskCacheInvalidator(workDir))
//	poller.StartAsync()
//
// State is persisted to ~/.xplat/cache/syncgh-poll-state.json
//
// # Task Cache Invalidation
//
// When syncgh detects changes, it can automatically invalidate the Task
// remote taskfile cache (.task/remote/):
//
//	// Option 1: Use the built-in callback
//	poller.OnChange(syncgh.TaskCacheInvalidator(workDir))
//
//	// Option 2: Manual invalidation
//	syncgh.InvalidateTaskCache(workDir, true) // aggressive=true clears all
//
// # Webhook Usage
//
// The Webhook server listens for GitHub webhook events:
//
//	server := syncgh.NewWebhookServer("8080")
//	server.Run()  // Listens on /webhook and /health
//
// # SSE Server Usage (gosmee-compatible)
//
// The SSE server receives webhooks and broadcasts them via SSE to connected clients.
// This is compatible with gosmee and smee.io protocols:
//
//	server := syncgh.NewSSEServer(syncgh.SSEServerConfig{
//	    Port:           "3333",
//	    PublicURL:      "https://webhook.example.com",
//	    WebhookSecrets: []string{"secret1", "secret2"},
//	})
//	server.Run()
//
// Endpoints:
//   - GET  /health           Health check
//   - GET  /new              Generate new channel URL
//   - GET  /events/{channel} SSE event stream
//   - POST /{channel}        Receive webhooks
//   - GET  /{channel}        Channel info page
//
// # SSE Client Usage
//
// The SSE client connects to a gosmee/SSE server and forwards events to a local target:
//
//	client := syncgh.NewSSEClient(syncgh.SSEClientConfig{
//	    ServerURL:    "https://webhook.example.com/abc123",
//	    TargetURL:    "http://localhost:8763/webhook",
//	    SaveDir:      "./webhooks",      // Save payloads for replay
//	    IgnoreEvents: []string{"ping"},  // Skip these event types
//	    HealthPort:   8080,               // K8s health probe endpoint
//	})
//	client.Run(ctx)
//
// Or use the convenience function with Task cache invalidation:
//
//	syncgh.RunSSEClientWithInvalidation(serverURL, workDir, port, saveDir, ignoreEvents, healthPort)
//
// # Replay Usage (Catch-up Missed Webhooks)
//
// The Replayer fetches past webhook deliveries from GitHub API and replays them:
//
//	replayer := syncgh.NewReplayer(syncgh.ReplayConfig{
//	    Owner:     "owner",
//	    Repo:      "repo",
//	    HookID:    12345,
//	    TargetURL: "http://localhost:8763/webhook",
//	    Since:     time.Now().Add(-24 * time.Hour),
//	    SaveDir:   "./webhooks",
//	})
//	replayer.Replay(ctx)
//
// List hooks and deliveries:
//
//	hooks, _ := replayer.ListHooks(ctx)
//	deliveries, _ := replayer.ListDeliveries(ctx, hookID)
//
// # Tunnel Usage (Development)
//
// For local development, use smee.io to forward webhooks:
//
//	// Generate a new smee.io channel
//	url := syncgh.GenerateSmeeChannel()
//
//	// Forward events to local server
//	syncgh.RunTunnel(smeeURL, "http://localhost:8080/webhook")
//
// # Design Notes
//
// Two poller options for different use cases:
//
// Poller (basic):
//   - Returns the new commit hash but does NOT track state
//   - oldVersion parameter is always empty
//   - Use when caller manages their own state
//
// StatefulPoller (recommended):
//   - Wraps Poller with PollState for persistence
//   - OnChange only fires when commit hash actually differs
//   - State saved to ~/.xplat/cache/syncgh-poll-state.json
//   - Use for Task cache invalidation, notifications, etc.
//
// Rate limiting:
//   - With GITHUB_TOKEN: 5000 requests/hour
//   - Without token: 60 requests/hour
//
// # Repo Auto-Discovery
//
// DiscoverReposFromProject scans Taskfile.yml files for remote includes
// and extracts the GitHub repos to watch:
//
//	repos, err := syncgh.DiscoverReposFromProject(workDir)
//	configs := syncgh.DiscoverReposToConfigs(repos)
//	poller, _ := syncgh.NewStatefulPoller(5*time.Minute, configs, token)
//
// Supported URL patterns:
//   - https://raw.githubusercontent.com/owner/repo/branch/path
//   - https://github.com/owner/repo.git//path
//
// # CLI Commands
//
//	xplat sync-gh discover               # Show repos from Taskfile.yml
//	xplat sync-gh poll                   # Poll (auto-discover repos)
//	xplat sync-gh poll --repos=owner/repo  # Poll specific repos
//	xplat sync-gh poll-state             # Show tracked commit hashes
//	xplat sync-gh webhook --port=8080    # Start webhook server
//	xplat sync-gh tunnel <smee-url>      # Forward smee.io events locally
//	xplat sync-gh tunnel-setup <repo>    # Create smee channel + GitHub webhook
//	xplat sync-gh state <owner/repo>     # Capture and save repo state
//	xplat sync-gh release <owner/repo>   # Get latest release tag
//	xplat sync-gh server                 # Start gosmee-compatible SSE server
//	xplat sync-gh sse-client <url>       # Connect to SSE server and forward events
//	xplat sync-gh replay owner/repo --list-hooks  # List webhooks
//	xplat sync-gh replay owner/repo 123 --list-deliveries  # List deliveries
//	xplat sync-gh replay owner/repo 123 http://localhost:8763/webhook  # Replay
package syncgh
