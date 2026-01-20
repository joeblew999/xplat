package syncgh

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/cbrgm/githubevents/v2/githubevents"
	"github.com/google/go-github/v80/github"
)

// WebhookConfig holds configuration for the webhook server
type WebhookConfig struct {
	Port       string
	WorkDir    string // Working directory for Task cache invalidation
	Invalidate bool   // Enable Task cache invalidation on push events
}

// WebhookServer handles GitHub webhook events
type WebhookServer struct {
	handler *githubevents.EventHandler
	port    string
	config  WebhookConfig
}

// NewWebhookServer creates a new webhook server
func NewWebhookServer(port string) *WebhookServer {
	return NewWebhookServerWithConfig(WebhookConfig{Port: port})
}

// NewWebhookServerWithConfig creates a new webhook server with full configuration
func NewWebhookServerWithConfig(config WebhookConfig) *WebhookServer {
	if config.Port == "" {
		config.Port = "8080"
	}

	handler := githubevents.New("")

	server := &WebhookServer{
		handler: handler,
		port:    config.Port,
		config:  config,
	}

	// Log ALL events
	handler.OnBeforeAny(func(ctx context.Context, deliveryID string, eventName string, event interface{}) error {
		log.Printf("Event: %s [delivery: %s]", eventName, deliveryID)
		return nil
	})

	// Handle push events - this is where real-time sync happens
	handler.OnPushEventAny(func(ctx context.Context, deliveryID string, eventName string, event *github.PushEvent) error {
		repo := event.GetRepo().GetFullName()
		ref := event.GetRef() // e.g., "refs/heads/main"

		// Extract branch name from ref
		branch := strings.TrimPrefix(ref, "refs/heads/")

		beforeSHA := event.GetBefore()
		afterSHA := event.GetAfter()
		if len(beforeSHA) > 8 {
			beforeSHA = beforeSHA[:8]
		}
		if len(afterSHA) > 8 {
			afterSHA = afterSHA[:8]
		}

		log.Printf("Push: %s@%s (%s -> %s)", repo, branch, beforeSHA, afterSHA)

		// Invalidate Task cache if enabled
		if server.config.Invalidate && server.config.WorkDir != "" {
			log.Printf("Invalidating Task cache for %s...", server.config.WorkDir)
			callback := TaskCacheInvalidator(server.config.WorkDir)
			callback(repo, branch, beforeSHA, afterSHA)
		}

		return nil
	})

	// Handle release events
	handler.OnReleaseEventAny(func(ctx context.Context, deliveryID string, eventName string, event *github.ReleaseEvent) error {
		repo := event.GetRepo().GetFullName()
		release := event.GetRelease()
		action := event.GetAction()

		log.Printf("Release: %s [%s] %s", repo, action, release.GetTagName())

		// Invalidate cache on release publish
		if action == "published" && server.config.Invalidate && server.config.WorkDir != "" {
			log.Printf("Invalidating Task cache for release %s...", release.GetTagName())
			callback := TaskCacheInvalidator(server.config.WorkDir)
			callback(repo, release.GetTagName(), "", release.GetTagName())
		}

		return nil
	})

	return server
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
	_, _ = fmt.Fprintf(w, "OK")
}

// Run starts the webhook server
func (s *WebhookServer) Run() error {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "OK")
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

// RunWebhookWithInvalidation starts a webhook server that invalidates Task cache on push events
func RunWebhookWithInvalidation(port, workDir string) {
	server := NewWebhookServerWithConfig(WebhookConfig{
		Port:       port,
		WorkDir:    workDir,
		Invalidate: true,
	})
	log.Printf("Task cache invalidation enabled for: %s", workDir)
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
