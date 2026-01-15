// Package syncgh provides GitHub sync operations.
//
// This file implements a gosmee-compatible SSE server for webhook relay.
// Pattern adopted from github.com/chmouel/gosmee for interoperability.
package syncgh

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// MaxChannelLength prevents DoS via long channel names.
	MaxChannelLength = 64
	// MaxBodySize limits webhook payload size (25MB default).
	MaxBodySize = 25 * 1024 * 1024
)

// SSEServerConfig holds configuration for the SSE server.
type SSEServerConfig struct {
	// Port to listen on (default "3333")
	Port string

	// PublicURL is the external URL for webhook configuration (optional)
	PublicURL string

	// WebhookSecrets for signature validation (optional, supports multiple)
	WebhookSecrets []string

	// OnEvent callback when webhook is received (optional)
	OnEvent func(channel, eventType, deliveryID string)
}

// EventBroker manages SSE subscriptions and publications.
// Pattern from gosmee for fan-out to multiple clients per channel.
type EventBroker struct {
	sync.RWMutex
	subscribers map[string][]*Subscriber
}

// Subscriber represents a client connection listening for events.
type Subscriber struct {
	Channel string
	Events  chan []byte
}

// NewEventBroker creates a new event broker.
func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: make(map[string][]*Subscriber),
	}
}

// Subscribe adds a subscriber for a specific channel.
func (eb *EventBroker) Subscribe(channel string) *Subscriber {
	eb.Lock()
	defer eb.Unlock()

	subscriber := &Subscriber{
		Channel: channel,
		Events:  make(chan []byte, 100), // Buffer to prevent blocking
	}

	eb.subscribers[channel] = append(eb.subscribers[channel], subscriber)
	return subscriber
}

// Unsubscribe removes a subscriber from a channel.
func (eb *EventBroker) Unsubscribe(channel string, subscriber *Subscriber) {
	eb.Lock()
	defer eb.Unlock()

	subscribers := eb.subscribers[channel]
	for i, s := range subscribers {
		if s == subscriber {
			eb.subscribers[channel] = slices.Delete(subscribers, i, i+1)
			close(subscriber.Events)
			break
		}
	}
}

// Publish sends an event to all subscribers of a channel.
func (eb *EventBroker) Publish(channel string, data []byte) {
	eb.RLock()
	subscribers := eb.subscribers[channel]
	eb.RUnlock()

	for _, s := range subscribers {
		// Non-blocking send
		select {
		case s.Events <- data:
		default:
			// Buffer full, skip this subscriber
		}
	}
}

// SubscriberCount returns the number of subscribers for a channel.
func (eb *EventBroker) SubscriberCount(channel string) int {
	eb.RLock()
	defer eb.RUnlock()
	return len(eb.subscribers[channel])
}

// SSEServer is a gosmee-compatible webhook relay server.
type SSEServer struct {
	config SSEServerConfig
	broker *EventBroker
}

// NewSSEServer creates a new SSE server.
func NewSSEServer(config SSEServerConfig) *SSEServer {
	if config.Port == "" {
		config.Port = "3333"
	}
	return &SSEServer{
		config: config,
		broker: NewEventBroker(),
	}
}

// Run starts the SSE server.
func (s *SSEServer) Run() error {
	mux := http.NewServeMux()

	// Health/version endpoint
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /version", s.handleHealth)

	// Generate new channel URL
	mux.HandleFunc("GET /new", s.handleNewChannel)

	// SSE events endpoint (gosmee pattern: /events/{channel})
	mux.HandleFunc("GET /events/", s.handleSSE)

	// Webhook POST endpoint (gosmee pattern: /{channel})
	mux.HandleFunc("POST /", s.handleWebhook)

	// Index page (optional, shows channel info)
	mux.HandleFunc("GET /", s.handleIndex)

	addr := ":" + s.config.Port
	publicURL := s.config.PublicURL
	if publicURL == "" {
		publicURL = fmt.Sprintf("http://localhost%s", addr)
	}

	log.Printf("SSE Server listening on %s", addr)
	log.Printf("Webhook URL: %s/<channel>", publicURL)
	log.Printf("SSE events: %s/events/<channel>", publicURL)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}

// handleHealth returns server health status.
func (s *SSEServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Gosmee-Version", "xplat-1.0")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "xplat-1.0",
	})
}

// handleNewChannel generates a new random channel URL.
func (s *SSEServer) handleNewChannel(w http.ResponseWriter, r *http.Request) {
	channel := randomChannelID()
	publicURL := s.config.PublicURL
	if publicURL == "" {
		publicURL = fmt.Sprintf("http://%s", r.Host)
	}
	url := fmt.Sprintf("%s/%s", publicURL, channel)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, url)
}

// handleIndex shows channel information.
func (s *SSEServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.URL.Path, "/")
	if channel == "" {
		// Redirect to new channel
		channel = randomChannelID()
		http.Redirect(w, r, "/"+channel, http.StatusFound)
		return
	}

	// Validate channel
	if len(channel) < 12 || len(channel) > MaxChannelLength {
		http.Error(w, "Invalid channel", http.StatusBadRequest)
		return
	}

	publicURL := s.config.PublicURL
	if publicURL == "" {
		publicURL = fmt.Sprintf("http://%s", r.Host)
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>xplat SSE Server - %s</title></head>
<body>
<h1>Webhook Relay Channel</h1>
<p><strong>Webhook URL:</strong> <code>%s/%s</code></p>
<p><strong>SSE Events:</strong> <code>%s/events/%s</code></p>
<p>Subscribers: %d</p>
<h2>Recent Events</h2>
<pre id="events"></pre>
<script>
const es = new EventSource('/events/%s');
es.onmessage = e => {
  const pre = document.getElementById('events');
  pre.textContent = e.data + '\n' + pre.textContent.slice(0, 10000);
};
es.onerror = () => console.log('SSE error, reconnecting...');
</script>
</body>
</html>`, channel, publicURL, channel, publicURL, channel, s.broker.SubscriberCount(channel), channel)
}

// handleSSE handles SSE client connections.
func (s *SSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Extract channel from /events/{channel}
	channel := strings.TrimPrefix(r.URL.Path, "/events/")
	if channel == "" || len(channel) < 12 || len(channel) > MaxChannelLength {
		http.Error(w, "Invalid channel", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send connected message (gosmee format)
	fmt.Fprintf(w, "data: %s\n\n", `{"message":"connected"}`)
	flusher.Flush()

	// Subscribe to channel
	subscriber := s.broker.Subscribe(channel)
	defer s.broker.Unsubscribe(channel, subscriber)

	// Send ready message
	fmt.Fprintf(w, "data: %s\n\n", `{"message":"ready"}`)
	flusher.Flush()

	log.Printf("SSE: Client connected to channel %s", channel)

	// Keepalive ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Event loop
	for {
		select {
		case <-r.Context().Done():
			log.Printf("SSE: Client disconnected from channel %s", channel)
			return

		case data, ok := <-subscriber.Events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case <-ticker.C:
			// Keepalive
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// handleWebhook receives webhooks and broadcasts via SSE.
func (s *SSEServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Extract channel from /{channel}
	channel := strings.TrimPrefix(r.URL.Path, "/")
	if channel == "" || len(channel) < 12 || len(channel) > MaxChannelLength {
		http.Error(w, "Invalid channel", http.StatusBadRequest)
		return
	}

	// Check content type
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	// Read body with size limit
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate webhook signature if secrets configured
	if len(s.config.WebhookSecrets) > 0 {
		if !s.validateSignature(body, r) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Verify it's valid JSON
	var d any
	if err := json.Unmarshal(body, &d); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Build SSE payload (gosmee format)
	now := time.Now().UTC()
	payload := make(map[string]any)

	// Copy headers (lowercase, gosmee pattern)
	for k, v := range r.Header {
		payload[strings.ToLower(k)] = v[0]
	}

	// Add timestamp and base64-encoded body
	payload["timestamp"] = fmt.Sprintf("%d", now.UnixMilli())
	payload["bodyB"] = base64.StdEncoding.EncodeToString(body)

	encoded, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast to subscribers
	s.broker.Publish(channel, encoded)

	// Call optional callback
	if s.config.OnEvent != nil {
		eventType := r.Header.Get("X-GitHub-Event")
		deliveryID := r.Header.Get("X-GitHub-Delivery")
		s.config.OnEvent(channel, eventType, deliveryID)
	}

	// Log
	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	log.Printf("SSE: Published %s event [%s] to channel %s (%d subscribers)",
		eventType, deliveryID, channel, s.broker.SubscriberCount(channel))

	// Response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Gosmee-Version", "xplat-1.0")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status":  http.StatusAccepted,
		"channel": channel,
		"message": "ok",
		"version": "xplat-1.0",
	})
}

// validateSignature validates webhook signatures for GitHub, GitLab, Bitbucket, Gitea.
func (s *SSEServer) validateSignature(body []byte, r *http.Request) bool {
	// GitLab uses X-Gitlab-Token (simple string comparison)
	if token := r.Header.Get("X-Gitlab-Token"); token != "" {
		for _, secret := range s.config.WebhookSecrets {
			if token == secret {
				return true
			}
		}
		return false
	}

	// GitHub uses X-Hub-Signature-256 (HMAC-SHA256)
	if sig := r.Header.Get("X-Hub-Signature-256"); sig != "" {
		return s.validateHMAC(body, sig, "sha256=")
	}

	// Bitbucket uses X-Hub-Signature (HMAC-SHA256 without prefix)
	if sig := r.Header.Get("X-Hub-Signature"); sig != "" {
		return s.validateHMAC(body, sig, "")
	}

	// Gitea uses X-Gitea-Signature
	if sig := r.Header.Get("X-Gitea-Signature"); sig != "" {
		return s.validateHMAC(body, sig, "sha256=")
	}

	return false
}

// validateHMAC validates HMAC-SHA256 signature.
func (s *SSEServer) validateHMAC(body []byte, signature, prefix string) bool {
	sig := strings.TrimPrefix(signature, prefix)
	for _, secret := range s.config.WebhookSecrets {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return true
		}
	}
	return false
}

// randomChannelID generates a random channel ID (12 chars).
func randomChannelID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 12)
	rand.Read(b)
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

// RunSSEServer starts the SSE server with the given configuration.
func RunSSEServer(port string, publicURL string, webhookSecrets []string) error {
	server := NewSSEServer(SSEServerConfig{
		Port:           port,
		PublicURL:      publicURL,
		WebhookSecrets: webhookSecrets,
	})
	return server.Run()
}
