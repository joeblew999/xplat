// Package synccf provides Cloudflare sync operations without requiring the wrangler CLI.
//
// This package enables xplat to integrate with Cloudflare services for:
//   - Tunnel management (cloudflared quick tunnels)
//   - Webhook handling (Cloudflare notifications)
//   - Audit log polling
//   - Authentication
//
// # Components
//
//   - Client: Main Cloudflare API client with event handling
//   - Tunnel: Manage cloudflared tunnels (quick tunnels or named)
//   - WebhookHandler: HTTP handler for Cloudflare notification webhooks
//   - AuditPoller: Poll Cloudflare audit logs for changes
//   - Auth: Authentication helpers for Cloudflare API
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
//   - CF_API_TOKEN: Cloudflare API token
//   - CF_ACCOUNT_ID: Cloudflare account ID
//
// # CLI Commands
//
//	xplat sync-cf tunnel --port=8080     # Start quick tunnel
//	xplat sync-cf webhook --port=8080    # Start webhook server
//	xplat sync-cf audit                  # Poll audit logs
package synccf
