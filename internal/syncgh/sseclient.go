// Package syncgh provides GitHub sync operations.
package syncgh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SSEClientConfig holds configuration for the SSE client.
// Patterns adopted from gosmee for improved reliability.
type SSEClientConfig struct {
	// ServerURL is the gosmee server URL (e.g., "https://webhook.example.com/channel123")
	ServerURL string

	// TargetURL is the local webhook handler URL (e.g., "http://localhost:8763/webhook")
	TargetURL string

	// SaveDir saves webhook payloads to disk for debugging/replay (optional)
	SaveDir string

	// IgnoreEvents skips these event types (e.g., ["ping", "status"])
	IgnoreEvents []string

	// HealthPort exposes a health endpoint for K8s probes (0 = disabled)
	HealthPort int

	// OnEvent is called for each webhook event received (optional, for logging/debugging)
	OnEvent func(eventType, deliveryID string)
}

// SSEClient connects to a gosmee server via SSE and forwards events to a local webhook handler.
type SSEClient struct {
	config     SSEClientConfig
	client     *http.Client
	retryCount int
}

// NewSSEClient creates a new SSE client.
func NewSSEClient(config SSEClientConfig) *SSEClient {
	return &SSEClient{
		config: config,
		client: &http.Client{
			Timeout: 0, // No timeout for SSE connections
		},
	}
}

// sseMessage represents a parsed SSE message from the gosmee server.
type sseMessage struct {
	// Headers from the original webhook request
	Headers map[string]string

	// Body is the decoded webhook payload
	Body []byte

	// EventType (e.g., "push", "release")
	EventType string

	// DeliveryID (X-GitHub-Delivery header)
	DeliveryID string

	// ContentType of the original request
	ContentType string

	// Timestamp of when the event was received
	Timestamp time.Time
}

// parseSSEData parses the SSE data from gosmee server.
// gosmee encodes the body as base64 in the "bodyB" field.
func parseSSEData(data []byte) (*sseMessage, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse SSE data: %w", err)
	}

	msg := &sseMessage{
		Headers:   make(map[string]string),
		Timestamp: time.Now().UTC(),
	}

	for key, value := range payload {
		strVal, ok := value.(string)
		if !ok {
			continue
		}

		switch key {
		case "bodyB":
			// Base64 encoded body
			decoded, err := base64.StdEncoding.DecodeString(strVal)
			if err != nil {
				return nil, fmt.Errorf("failed to decode bodyB: %w", err)
			}
			msg.Body = decoded

		case "body":
			// Already decoded body (fallback)
			if msg.Body == nil {
				msg.Body = []byte(strVal)
			}

		case "x-github-event":
			msg.EventType = strVal
			msg.Headers["X-GitHub-Event"] = strVal

		case "x-github-delivery":
			msg.DeliveryID = strVal
			msg.Headers["X-GitHub-Delivery"] = strVal

		case "content-type":
			msg.ContentType = strVal
			msg.Headers["Content-Type"] = strVal

		case "timestamp":
			// Skip timestamp

		default:
			// Include other headers (x-* headers)
			if strings.HasPrefix(key, "x-") || key == "user-agent" {
				// Convert to proper header format (e.g., x-github-event -> X-GitHub-Event)
				headerKey := headerize(key)
				msg.Headers[headerKey] = strVal
			}
		}
	}

	return msg, nil
}

// headerize converts a lowercase header key to proper HTTP header format.
func headerize(key string) string {
	parts := strings.Split(key, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "-")
}

// forwardToTarget sends the webhook event to the local target URL.
func (c *SSEClient) forwardToTarget(msg *sseMessage) error {
	req, err := http.NewRequest(http.MethodPost, c.config.TargetURL, bytes.NewReader(msg.Body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range msg.Headers {
		req.Header.Set(k, v)
	}

	// Ensure content-type is set
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward to target: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("target returned error: %d %s", resp.StatusCode, string(body))
	}

	return nil
}

// savePayload saves the webhook payload to disk for debugging/replay.
// Follows gosmee's pattern: creates JSON payload + shell script for replay.
func (c *SSEClient) savePayload(msg *sseMessage) error {
	if c.config.SaveDir == "" {
		return nil
	}

	// Create save directory if needed
	if err := os.MkdirAll(c.config.SaveDir, 0o755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	// Build filename: eventtype-timestamp or just timestamp
	ts := msg.Timestamp.Format("2006-01-02T15.04.05.000")
	baseName := ts
	if msg.EventType != "" {
		baseName = fmt.Sprintf("%s-%s", msg.EventType, ts)
	}

	// Save JSON payload
	jsonPath := filepath.Join(c.config.SaveDir, baseName+".json")
	if err := os.WriteFile(jsonPath, msg.Body, 0o644); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	// Save replay shell script
	shPath := filepath.Join(c.config.SaveDir, baseName+".sh")
	script := c.buildReplayScript(msg, baseName)
	if err := os.WriteFile(shPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("failed to write shell script: %w", err)
	}

	log.Printf("SSE: Saved payload to %s", jsonPath)
	return nil
}

// buildReplayScript creates a curl command to replay the webhook.
func (c *SSEClient) buildReplayScript(msg *sseMessage, baseName string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# Replay webhook event\n")
	sb.WriteString("# Generated by xplat sync-gh sse-client\n\n")

	sb.WriteString("TARGET_URL=\"${1:-")
	sb.WriteString(c.config.TargetURL)
	sb.WriteString("}\"\n\n")

	sb.WriteString("curl -X POST \"$TARGET_URL\" \\\n")

	// Add headers
	for k, v := range msg.Headers {
		sb.WriteString(fmt.Sprintf("  -H '%s: %s' \\\n", k, v))
	}

	sb.WriteString(fmt.Sprintf("  -d @\"$(dirname \"$0\")/%s.json\"\n", baseName))

	return sb.String()
}

// calculateBackoff returns the backoff duration using exponential backoff.
// Pattern from gosmee: starts at 1s, doubles each retry, caps at 60s.
func (c *SSEClient) calculateBackoff() time.Duration {
	// Exponential backoff: 1s, 2s, 4s, 8s, 16s, 32s, 60s (cap)
	backoff := time.Duration(1<<c.retryCount) * time.Second
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}
	c.retryCount++
	return backoff
}

// resetBackoff resets the retry counter after a successful connection.
func (c *SSEClient) resetBackoff() {
	c.retryCount = 0
}

// Run connects to the SSE server and forwards events to the target.
// This function blocks until the context is cancelled or an error occurs.
// Uses exponential backoff for reconnection (gosmee pattern).
func (c *SSEClient) Run(ctx context.Context) error {
	log.Printf("SSE client connecting to %s", c.config.ServerURL)
	log.Printf("Forwarding events to %s", c.config.TargetURL)

	if c.config.SaveDir != "" {
		log.Printf("Saving payloads to %s", c.config.SaveDir)
	}

	if len(c.config.IgnoreEvents) > 0 {
		log.Printf("Ignoring events: %v", c.config.IgnoreEvents)
	}

	// Start health server if configured
	if c.config.HealthPort > 0 {
		c.startHealthServer()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			backoff := c.calculateBackoff()
			log.Printf("SSE connection error: %v, reconnecting in %v...", err, backoff)
			time.Sleep(backoff)
		}
	}
}

// startHealthServer starts a health endpoint for K8s probes.
// Pattern from gosmee's serveHealthEndpoint.
func (c *SSEClient) startHealthServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","server":"%s"}`, c.config.ServerURL)
	})

	addr := fmt.Sprintf(":%d", c.config.HealthPort)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("SSE: Health server listening on %s", addr)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("SSE: Health server error: %v", err)
		}
	}()
}

// connect establishes a single SSE connection and processes events.
func (c *SSEClient) connect(ctx context.Context) error {
	// Build the SSE events URL
	// gosmee expects /events/{channel} endpoint
	sseURL := c.config.ServerURL
	if !strings.Contains(sseURL, "/events/") {
		// Extract channel from URL and construct events URL
		parts := strings.Split(sseURL, "/")
		if len(parts) > 0 {
			channel := parts[len(parts)-1]
			baseURL := strings.TrimSuffix(sseURL, "/"+channel)
			sseURL = fmt.Sprintf("%s/events/%s", baseURL, channel)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("User-Agent", "xplat-sse-client/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SSE server returned %d: %s", resp.StatusCode, string(body))
	}

	// Connection successful, reset backoff
	c.resetBackoff()
	log.Printf("SSE client connected, waiting for events...")

	scanner := bufio.NewScanner(resp.Body)
	var dataBuffer bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments (keepalives)
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Empty line signals end of message
		if line == "" {
			if dataBuffer.Len() > 0 {
				c.processEvent(dataBuffer.Bytes())
				dataBuffer.Reset()
			}
			continue
		}

		// Parse SSE field
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			dataBuffer.WriteString(data)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE read error: %w", err)
	}

	return fmt.Errorf("SSE connection closed")
}

// processEvent handles a single SSE event.
func (c *SSEClient) processEvent(data []byte) {
	// Skip empty or connection messages
	if len(data) == 0 || string(data) == "{}" {
		return
	}

	// Skip connection/ready messages
	dataStr := strings.ToLower(string(data))
	if strings.Contains(dataStr, `"message"`) && (strings.Contains(dataStr, `"connected"`) || strings.Contains(dataStr, `"ready"`)) {
		log.Printf("SSE: Ready to receive events")
		return
	}

	// Parse the event
	msg, err := parseSSEData(data)
	if err != nil {
		log.Printf("SSE: Failed to parse event: %v", err)
		return
	}

	// Skip if no body
	if len(msg.Body) == 0 {
		return
	}

	// Check if event should be ignored (gosmee pattern)
	if len(c.config.IgnoreEvents) > 0 && msg.EventType != "" {
		if slices.Contains(c.config.IgnoreEvents, msg.EventType) {
			log.Printf("SSE: Skipping ignored event type: %s", msg.EventType)
			return
		}
	}

	// Call optional callback
	if c.config.OnEvent != nil {
		c.config.OnEvent(msg.EventType, msg.DeliveryID)
	}

	// Log the event
	log.Printf("SSE: Received %s event [%s]", msg.EventType, msg.DeliveryID)

	// Save payload if configured (gosmee pattern)
	if c.config.SaveDir != "" {
		if err := c.savePayload(msg); err != nil {
			log.Printf("SSE: Failed to save payload: %v", err)
		}
	}

	// Forward to target
	if err := c.forwardToTarget(msg); err != nil {
		log.Printf("SSE: Failed to forward event: %v", err)
	} else {
		log.Printf("SSE: Forwarded to %s", c.config.TargetURL)
	}
}

// RunSSEClient starts the SSE client with the given configuration.
// This is a convenience function for CLI usage.
func RunSSEClient(serverURL, targetURL string) error {
	client := NewSSEClient(SSEClientConfig{
		ServerURL: serverURL,
		TargetURL: targetURL,
	})

	return client.Run(context.Background())
}

// RunSSEClientWithOptions starts the SSE client with full configuration options.
func RunSSEClientWithOptions(ctx context.Context, config SSEClientConfig) error {
	client := NewSSEClient(config)
	return client.Run(ctx)
}

// RunSSEClientWithInvalidation starts the SSE client and also runs a local webhook
// server that invalidates Task cache on push events.
//
// This combines:
//  1. SSE client connecting to gosmee server
//  2. Local webhook handler that parses GitHub events
//  3. Task cache invalidation on detected changes
func RunSSEClientWithInvalidation(serverURL, workDir string, port string, saveDir string, ignoreEvents []string, healthPort int) error {
	if port == "" {
		port = "8763"
	}
	targetURL := fmt.Sprintf("http://localhost:%s/webhook", port)

	// Start the webhook server with cache invalidation in background
	go func() {
		log.Printf("Starting local webhook handler on port %s with cache invalidation", port)
		RunWebhookWithInvalidation(port, workDir)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Start SSE client with gosmee patterns
	client := NewSSEClient(SSEClientConfig{
		ServerURL:    serverURL,
		TargetURL:    targetURL,
		SaveDir:      saveDir,
		IgnoreEvents: ignoreEvents,
		HealthPort:   healthPort,
	})

	return client.Run(context.Background())
}
