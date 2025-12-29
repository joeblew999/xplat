package syncgh

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/cbrgm/githubevents/v2/githubevents"
)

// WebhookServer handles GitHub webhook events
type WebhookServer struct {
	handler *githubevents.EventHandler
	port    string
}

// NewWebhookServer creates a new webhook server
func NewWebhookServer(port string) *WebhookServer {
	if port == "" {
		port = "8080"
	}

	handler := githubevents.New("")

	// Log ALL events - we'll decide what to do with them later
	handler.OnBeforeAny(func(ctx context.Context, deliveryID string, eventName string, event interface{}) error {
		log.Printf("Event: %s [delivery: %s]", eventName, deliveryID)
		return nil
	})

	return &WebhookServer{
		handler: handler,
		port:    port,
	}
}

// HandleWebhook processes incoming webhook requests
func (s *WebhookServer) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	err := s.handler.HandleEventRequest(r)
	if err != nil {
		log.Printf("Webhook error: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// Run starts the webhook server
func (s *WebhookServer) Run() error {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	http.HandleFunc("/webhook", s.HandleWebhook)
	http.HandleFunc("/webhook/", s.HandleWebhook)

	addr := fmt.Sprintf(":%s", s.port)
	log.Printf("Webhook server listening on %s", addr)

	return http.ListenAndServe(addr, nil)
}

// RunWebhook starts a standalone webhook server on the specified port
func RunWebhook(port string) {
	server := NewWebhookServer(port)
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
