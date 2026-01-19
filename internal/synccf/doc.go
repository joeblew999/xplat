// Package synccf provides Cloudflare sync operations without requiring the wrangler CLI.
//
// This package enables xplat to integrate with Cloudflare services for:
//   - Receiving events from CF Worker (round-trip validation)
//   - Tunnel management (cloudflared quick tunnels)
//   - Webhook handling (Cloudflare notifications)
//   - Audit log polling
//   - Authentication
//   - Task cache invalidation on Pages deploy
//
// # Components
//
//   - ReceiveHandler: Receives events forwarded by the CF Worker
//   - TaskCacheInvalidator: Callback to invalidate Task cache on deploy events
//   - Client: Main Cloudflare API client with event handling
//   - Tunnel: Manage cloudflared tunnels (quick tunnels or named)
//   - WebhookHandler: HTTP handler for Cloudflare notification webhooks
//   - AuditPoller: Poll Cloudflare audit logs for changes
//   - Auth: Authentication helpers for Cloudflare API
//
// # Round-Trip Validation (Recommended)
//
// The primary use case is round-trip validation of Cloudflare operations:
//
//  1. CF Worker receives events from Cloudflare services (Pages, Notifications)
//  2. Worker normalizes and forwards events to SYNC_ENDPOINT
//  3. ReceiveHandler processes events and triggers callbacks
//  4. TaskCacheInvalidator clears Task cache on Pages deploy events
//
// Architecture:
//
//	Cloudflare → CF Worker → Tunnel → ReceiveHandler → Callbacks
//
// # Receiver Usage
//
// Start a receiver to get events from the CF Worker:
//
//	synccf.RunReceiveServer("9091", synccf.ReceiveCallbacks{
//	    OnPagesDeploy: synccf.TaskCacheInvalidator(workDir),
//	    OnAny:         synccf.DefaultLogCallback(),
//	})
//
// # Tunnel Usage
//
// Create a quick tunnel to expose a local port:
//
//	tunnel := synccf.NewTunnel(synccf.TunnelConfig{
//	    LocalPort: 8080,
//	})
//	url, err := tunnel.Start(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Tunnel URL: %s\n", url)
//	defer tunnel.Stop()
//
// # Webhook Usage
//
// Handle incoming Cloudflare notification webhooks:
//
//	client := synccf.NewClient(synccf.Config{
//	    APIToken:  os.Getenv("CF_API_TOKEN"),
//	    AccountID: os.Getenv("CF_ACCOUNT_ID"),
//	})
//	handler := synccf.NewWebhookHandler(client, secretKey)
//	http.Handle("/webhook", handler)
//
// # Event Handling
//
// Register handlers for specific event types:
//
//	client.On(synccf.EventPagesDeploy, func(ctx context.Context, event synccf.Event) error {
//	    log.Printf("Pages deploy: %s", event.Resource)
//	    return nil
//	})
//
// # Audit Log Polling
//
// Poll Cloudflare audit logs for changes:
//
//	poller := synccf.NewAuditPoller(client, 5*time.Minute)
//	poller.OnEntry(func(entry synccf.AuditLogEntry) {
//	    log.Printf("Audit: %s by %s", entry.Action.Type, entry.Actor.Email)
//	})
//	poller.Start(ctx)
//
// # Environment Variables
//
// These can be set in your .env file (used by wizard and CLI):
//
//   - CLOUDFLARE_API_TOKEN: Cloudflare API token
//   - CLOUDFLARE_ACCOUNT_ID: Cloudflare account ID
//   - CLOUDFLARE_WORKER_NAME: Name of the sync Worker (default: xplat-sync)
//   - CLOUDFLARE_SYNC_ENDPOINT: Tunnel URL for Worker forwarding
//   - CLOUDFLARE_RECEIVER_PORT: Local receiver port (default: 9091)
//
// Legacy environment variables (still supported):
//
//   - CF_API_TOKEN: Cloudflare API token
//   - CF_ACCOUNT_ID: Cloudflare account ID
//
// # CLI Commands
//
//	xplat sync-cf receive --port=9091 --invalidate  # Receive Worker events with cache invalidation
//	xplat sync-cf receive-state                     # Show processed events state
//	xplat sync-cf tunnel --port=8080                # Start quick tunnel
//	xplat sync-cf webhook --port=8080               # Start webhook server
//	xplat sync-cf poll                              # Poll audit logs
//
// # Web UI Integration
//
// The Cloudflare setup wizard (internal/env/web/) includes Step 6 for event
// notifications setup. This step configures the Worker name, receiver port,
// and provides instructions for starting the receiver and tunnel.
//
// The CLI commands (sync-cf receive, sync-cf tunnel) automatically use the
// receiver port from .env when no --port flag is provided.
package synccf
