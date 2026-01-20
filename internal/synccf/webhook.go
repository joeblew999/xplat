package synccf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// AlertPayload represents a Cloudflare notification webhook payload
type AlertPayload struct {
	Name        string                 `json:"name"`
	Text        string                 `json:"text"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   string                 `json:"ts"`
	AccountID   string                 `json:"account_id"`
	AccountName string                 `json:"account_name"`
	AlertType   string                 `json:"alert_type"`
}

// WebhookHandler handles incoming Cloudflare webhook events
type WebhookHandler struct {
	client    *Client
	secretKey string
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(client *Client, secretKey string) *WebhookHandler {
	return &WebhookHandler{
		client:    client,
		secretKey: secretKey,
	}
}

// ServeHTTP handles incoming webhook requests
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("sync-cf webhook: failed to read body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var alert AlertPayload
	if err := json.Unmarshal(body, &alert); err != nil {
		log.Printf("sync-cf webhook: failed to parse payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	ts, err := time.Parse(time.RFC3339, alert.Timestamp)
	if err != nil {
		ts = time.Now()
	}

	eventType := EventAlert
	resource := alert.AlertType

	switch alert.AlertType {
	case "pages_event":
		eventType = EventPagesDeploy
		resource = "pages"
	case "workers_event":
		eventType = EventWorkersDeploy
		resource = "workers"
	case "tunnel_health_event":
		eventType = EventTunnel
		resource = "tunnel"
	}

	event := Event{
		Type:      eventType,
		Timestamp: ts,
		AccountID: alert.AccountID,
		Action:    alert.Name,
		Resource:  resource,
		Metadata: map[string]interface{}{
			"text":         alert.Text,
			"alert_type":   alert.AlertType,
			"account_name": alert.AccountName,
			"data":         alert.Data,
		},
		Raw: alert,
	}

	log.Printf("sync-cf webhook: received %s event: %s", event.Type, event.Action)
	h.client.emit(r.Context(), event)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "OK")
}

// HandleWebhook returns an http.HandlerFunc for use with standard routers
func (c *Client) HandleWebhook(secretKey string) http.HandlerFunc {
	handler := NewWebhookHandler(c, secretKey)
	return handler.ServeHTTP
}

// LogpushHandler handles incoming Logpush webhook events
type LogpushHandler struct {
	client  *Client
	dataset string
}

// NewLogpushHandler creates a new Logpush handler
func NewLogpushHandler(client *Client, dataset string) *LogpushHandler {
	return &LogpushHandler{
		client:  client,
		dataset: dataset,
	}
}

// ServeHTTP handles incoming Logpush requests
func (h *LogpushHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("sync-cf logpush: failed to read body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var entries []map[string]interface{}
	if err := json.Unmarshal(body, &entries); err != nil {
		log.Printf("sync-cf logpush: failed to parse payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	log.Printf("sync-cf logpush: received %d entries for dataset %s", len(entries), h.dataset)

	event := Event{
		Type:      EventLogpush,
		Timestamp: time.Now(),
		AccountID: h.client.accountID,
		Action:    "logpush_batch",
		Resource:  h.dataset,
		Metadata: map[string]interface{}{
			"count":   len(entries),
			"dataset": h.dataset,
		},
		Raw: entries,
	}

	h.client.emit(r.Context(), event)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "OK")
}

// HandleLogpush returns an http.HandlerFunc for Logpush webhooks
func (c *Client) HandleLogpush(dataset string) http.HandlerFunc {
	handler := NewLogpushHandler(c, dataset)
	return handler.ServeHTTP
}

// RegisterRoutes registers all Cloudflare webhook routes on a mux
func (c *Client) RegisterRoutes(mux *http.ServeMux, basePath string, secretKey string) {
	mux.HandleFunc(basePath+"/webhook", c.HandleWebhook(secretKey))
	mux.HandleFunc(basePath+"/webhook/", c.HandleWebhook(secretKey))

	mux.HandleFunc(basePath+"/logpush/http_requests", c.HandleLogpush("http_requests"))
	mux.HandleFunc(basePath+"/logpush/firewall_events", c.HandleLogpush("firewall_events"))
	mux.HandleFunc(basePath+"/logpush/audit_logs", c.HandleLogpush("audit_logs"))
}

// RunWebhookServer starts a standalone webhook server
func RunWebhookServer(port string, accountID, apiToken, webhookSecret string) error {
	if port == "" {
		port = "9090"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "OK")
	})

	if accountID != "" && apiToken != "" {
		client, err := NewClient(Config{
			APIToken:  apiToken,
			AccountID: accountID,
		})
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		client.OnAny(func(ctx context.Context, event Event) error {
			log.Printf("EVENT: [%s] %s on %s", event.Type, event.Action, event.Resource)
			if event.Actor != "" {
				log.Printf("  Actor: %s", event.Actor)
			}
			return nil
		})

		client.RegisterRoutes(mux, "/cf", webhookSecret)
	}

	log.Printf("sync-cf webhook server listening on :%s", port)
	log.Printf("   Health: http://localhost:%s/health", port)
	log.Printf("   Webhook: http://localhost:%s/cf/webhook", port)

	return http.ListenAndServe(":"+port, mux)
}
