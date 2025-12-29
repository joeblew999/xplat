// Package main implements a Cloudflare Worker for event aggregation.
// This worker receives events from various Cloudflare sources and forwards
// them to your xplat sync service endpoint.
//
// Deploy: xplat task workers/sync-cf:deploy
// Dev: xplat task workers/sync-cf:run
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare/fetch"
)

// Event represents a normalized Cloudflare event
type Event struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	AccountID string                 `json:"account_id,omitempty"`
	ZoneID    string                 `json:"zone_id,omitempty"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Source    string                 `json:"source"` // Which CF service sent this
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Raw       json.RawMessage        `json:"raw,omitempty"`
}

// Usage tracks request counts for billing visibility
type Usage struct {
	mu              sync.Mutex
	TotalRequests   int64
	WebhookPages    int64
	WebhookAlert    int64
	Logpush         int64
	ForwardSuccess  int64
	ForwardFailures int64
}

func (u *Usage) incTotal()          { u.mu.Lock(); u.TotalRequests++; u.mu.Unlock() }
func (u *Usage) incPages()          { u.mu.Lock(); u.WebhookPages++; u.mu.Unlock() }
func (u *Usage) incAlert()          { u.mu.Lock(); u.WebhookAlert++; u.mu.Unlock() }
func (u *Usage) incLogpush()        { u.mu.Lock(); u.Logpush++; u.mu.Unlock() }
func (u *Usage) incForwardSuccess() { u.mu.Lock(); u.ForwardSuccess++; u.mu.Unlock() }
func (u *Usage) incForwardFailure() { u.mu.Lock(); u.ForwardFailures++; u.mu.Unlock() }

func (u *Usage) snapshot() map[string]int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return map[string]int64{
		"total_requests":   u.TotalRequests,
		"webhook_pages":    u.WebhookPages,
		"webhook_alert":    u.WebhookAlert,
		"logpush":          u.Logpush,
		"forward_success":  u.ForwardSuccess,
		"forward_failures": u.ForwardFailures,
	}
}

// Version set by ldflags at build time
var version = "dev"

// In-memory usage counters (reset on worker restart)
var usage Usage

// Config from environment variables
var (
	syncEndpoint string // Where to forward events (e.g., your tunnel URL)
	syncToken    string // Auth token for your sync service
	workerName   string // Worker name for identification
)

func init() {
	syncEndpoint = os.Getenv("SYNC_ENDPOINT")
	syncToken = os.Getenv("SYNC_TOKEN")
	workerName = os.Getenv("WORKER_NAME")
	if workerName == "" {
		workerName = "xplat-sync-cf"
	}
}

func main() {
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/metrics", handleMetrics)
	http.HandleFunc("/webhook/pages", handlePagesWebhook)
	http.HandleFunc("/webhook/alert", handleAlertWebhook)
	http.HandleFunc("/logpush", handleLogpush)

	workers.Serve(nil)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": workerName,
		"version": version,
		"endpoints": []string{
			"/health",
			"/metrics",
			"/webhook/pages",
			"/webhook/alert",
			"/logpush",
		},
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// handleMetrics returns usage metrics for billing visibility
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	metrics := map[string]interface{}{
		"worker":  workerName,
		"version": version,
		"usage":   usage.snapshot(),
		"config": map[string]interface{}{
			"sync_endpoint_configured": syncEndpoint != "",
		},
		"billing_note": "Cloudflare Workers: Free tier 100k req/day, Paid $5/mo + $0.50/million after 10M.",
	}

	json.NewEncoder(w).Encode(metrics)
}

// handlePagesWebhook handles Cloudflare Pages deploy hooks
func handlePagesWebhook(w http.ResponseWriter, r *http.Request) {
	usage.incTotal()
	usage.incPages()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	event := Event{
		Type:      "pages_deploy",
		Timestamp: time.Now(),
		Action:    "deploy",
		Resource:  "pages",
		Source:    "pages_deploy_hook",
		Metadata:  payload,
		Raw:       body,
	}

	if err := forwardEvent(r.Context(), event); err != nil {
		log.Printf("forward error: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// handleAlertWebhook handles Cloudflare Notifications webhooks
func handleAlertWebhook(w http.ResponseWriter, r *http.Request) {
	usage.incTotal()
	usage.incAlert()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload struct {
		Name        string                 `json:"name"`
		Text        string                 `json:"text"`
		Data        map[string]interface{} `json:"data"`
		Timestamp   string                 `json:"ts"`
		AccountID   string                 `json:"account_id"`
		AccountName string                 `json:"account_name"`
		AlertType   string                 `json:"alert_type"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	event := Event{
		Type:      mapAlertType(payload.AlertType),
		Timestamp: time.Now(),
		AccountID: payload.AccountID,
		Action:    payload.Name,
		Resource:  payload.AlertType,
		Source:    "notification_webhook",
		Metadata: map[string]interface{}{
			"text":         payload.Text,
			"account_name": payload.AccountName,
			"data":         payload.Data,
		},
		Raw: body,
	}

	if err := forwardEvent(r.Context(), event); err != nil {
		log.Printf("forward error: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// handleLogpush handles Cloudflare Logpush HTTP destination
func handleLogpush(w http.ResponseWriter, r *http.Request) {
	usage.incTotal()
	usage.incLogpush()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dataset := r.URL.Query().Get("dataset")
	if dataset == "" {
		dataset = "unknown"
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var entries []map[string]interface{}
	if err := json.Unmarshal(body, &entries); err != nil {
		log.Printf("logpush: parse error (may be NDJSON): %v", err)
	}

	event := Event{
		Type:      "logpush",
		Timestamp: time.Now(),
		Action:    "batch",
		Resource:  dataset,
		Source:    "logpush",
		Metadata: map[string]interface{}{
			"dataset": dataset,
			"count":   len(entries),
		},
		Raw: body,
	}

	if err := forwardEvent(r.Context(), event); err != nil {
		log.Printf("forward error: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// mapAlertType maps Cloudflare alert types to normalized event types
func mapAlertType(alertType string) string {
	switch alertType {
	case "pages_event":
		return "pages_deploy"
	case "workers_event":
		return "workers_deploy"
	case "tunnel_health_event":
		return "tunnel"
	default:
		return "alert"
	}
}

// forwardEvent sends the event to the sync service
func forwardEvent(ctx context.Context, event Event) error {
	if syncEndpoint == "" {
		log.Printf("SYNC_ENDPOINT not configured, event: %s/%s", event.Type, event.Action)
		return nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		usage.incForwardFailure()
		return fmt.Errorf("marshal event: %w", err)
	}

	cli := fetch.NewClient()
	req, err := fetch.NewRequest(ctx, http.MethodPost, syncEndpoint, bytes.NewReader(body))
	if err != nil {
		usage.incForwardFailure()
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if syncToken != "" {
		req.Header.Set("Authorization", "Bearer "+syncToken)
	}

	resp, err := cli.Do(req, nil)
	if err != nil {
		usage.incForwardFailure()
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		usage.incForwardFailure()
		return fmt.Errorf("sync service returned %d", resp.StatusCode)
	}

	usage.incForwardSuccess()
	log.Printf("forwarded event: %s/%s", event.Type, event.Action)
	return nil
}
