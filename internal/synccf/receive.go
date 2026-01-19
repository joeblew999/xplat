package synccf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/joeblew999/xplat/internal/config"
)

// WorkerEvent represents the normalized event format from sync-cf Worker.
// This matches the Event struct in workers/sync-cf/main.go
type WorkerEvent struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	AccountID string                 `json:"account_id,omitempty"`
	ZoneID    string                 `json:"zone_id,omitempty"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Source    string                 `json:"source"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Raw       json.RawMessage        `json:"raw,omitempty"`
}

// ReceiverState tracks processed events to avoid duplicates
type ReceiverState struct {
	UpdatedAt       time.Time                 `json:"updated_at"`
	LastEventTime   time.Time                 `json:"last_event_time"`
	ProcessedEvents map[string]ProcessedEvent `json:"processed_events"`
}

// ProcessedEvent stores info about a processed event
type ProcessedEvent struct {
	Type        string    `json:"type"`
	Action      string    `json:"action"`
	Resource    string    `json:"resource"`
	ProcessedAt time.Time `json:"processed_at"`
}

// ReceiveHandler receives events from the CF Worker
type ReceiveHandler struct {
	mu            sync.RWMutex
	onPagesDeploy func(ctx context.Context, event WorkerEvent) error
	onAlert       func(ctx context.Context, event WorkerEvent) error
	onLogpush     func(ctx context.Context, event WorkerEvent) error
	onAny         func(ctx context.Context, event WorkerEvent) error
	state         *ReceiverState
	statePath     string
}

// NewReceiveHandler creates a new receive handler
func NewReceiveHandler() *ReceiveHandler {
	statePath := filepath.Join(config.XplatCache(), "synccf-receive-state.json")
	state := &ReceiverState{
		ProcessedEvents: make(map[string]ProcessedEvent),
	}

	// Try to load existing state
	if data, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(data, state)
	}

	return &ReceiveHandler{
		state:     state,
		statePath: statePath,
	}
}

// OnPagesDeploy registers a callback for Pages deploy events
func (h *ReceiveHandler) OnPagesDeploy(fn func(ctx context.Context, event WorkerEvent) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onPagesDeploy = fn
}

// OnAlert registers a callback for alert events
func (h *ReceiveHandler) OnAlert(fn func(ctx context.Context, event WorkerEvent) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAlert = fn
}

// OnLogpush registers a callback for logpush events
func (h *ReceiveHandler) OnLogpush(fn func(ctx context.Context, event WorkerEvent) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onLogpush = fn
}

// OnAny registers a callback for all events
func (h *ReceiveHandler) OnAny(fn func(ctx context.Context, event WorkerEvent) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAny = fn
}

// ServeHTTP handles incoming events from the Worker
func (h *ReceiveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("sync-cf receive: failed to read body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var event WorkerEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("sync-cf receive: failed to parse event: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Generate event key for deduplication
	eventKey := fmt.Sprintf("%s:%s:%s:%d", event.Type, event.Action, event.Resource, event.Timestamp.Unix())

	h.mu.RLock()
	_, alreadyProcessed := h.state.ProcessedEvents[eventKey]
	h.mu.RUnlock()

	if alreadyProcessed {
		log.Printf("sync-cf receive: skipping duplicate event: %s", eventKey)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK (duplicate)")
		return
	}

	log.Printf("sync-cf receive: [%s] %s on %s (source: %s)", event.Type, event.Action, event.Resource, event.Source)

	// Dispatch to handlers
	ctx := r.Context()
	h.mu.RLock()
	onPagesDeploy := h.onPagesDeploy
	onAlert := h.onAlert
	onLogpush := h.onLogpush
	onAny := h.onAny
	h.mu.RUnlock()

	// Call type-specific handlers
	switch event.Type {
	case "pages_deploy":
		if onPagesDeploy != nil {
			if err := onPagesDeploy(ctx, event); err != nil {
				log.Printf("sync-cf receive: pages_deploy handler error: %v", err)
			}
		}
	case "alert":
		if onAlert != nil {
			if err := onAlert(ctx, event); err != nil {
				log.Printf("sync-cf receive: alert handler error: %v", err)
			}
		}
	case "logpush":
		if onLogpush != nil {
			if err := onLogpush(ctx, event); err != nil {
				log.Printf("sync-cf receive: logpush handler error: %v", err)
			}
		}
	}

	// Call any handler
	if onAny != nil {
		if err := onAny(ctx, event); err != nil {
			log.Printf("sync-cf receive: any handler error: %v", err)
		}
	}

	// Mark event as processed
	h.mu.Lock()
	h.state.ProcessedEvents[eventKey] = ProcessedEvent{
		Type:        event.Type,
		Action:      event.Action,
		Resource:    event.Resource,
		ProcessedAt: time.Now(),
	}
	h.state.LastEventTime = event.Timestamp
	h.state.UpdatedAt = time.Now()

	// Prune old events (keep last 1000)
	if len(h.state.ProcessedEvents) > 1000 {
		var oldest string
		var oldestTime time.Time
		for k, v := range h.state.ProcessedEvents {
			if oldest == "" || v.ProcessedAt.Before(oldestTime) {
				oldest = k
				oldestTime = v.ProcessedAt
			}
		}
		delete(h.state.ProcessedEvents, oldest)
	}
	h.mu.Unlock()

	// Save state
	h.saveState()

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func (h *ReceiveHandler) saveState() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(h.statePath), 0755)

	data, err := json.MarshalIndent(h.state, "", "  ")
	if err != nil {
		log.Printf("sync-cf receive: failed to marshal state: %v", err)
		return
	}

	if err := os.WriteFile(h.statePath, data, 0644); err != nil {
		log.Printf("sync-cf receive: failed to save state: %v", err)
	}
}

// GetState returns the current receiver state
func (h *ReceiveHandler) GetState() ReceiverState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return *h.state
}

// RunReceiveServer starts an HTTP server to receive Worker events
func RunReceiveServer(port string, callbacks ReceiveCallbacks) error {
	if port == "" {
		port = "9091"
	}

	handler := NewReceiveHandler()

	// Register callbacks
	if callbacks.OnPagesDeploy != nil {
		handler.OnPagesDeploy(callbacks.OnPagesDeploy)
	}
	if callbacks.OnAlert != nil {
		handler.OnAlert(callbacks.OnAlert)
	}
	if callbacks.OnLogpush != nil {
		handler.OnLogpush(callbacks.OnLogpush)
	}
	if callbacks.OnAny != nil {
		handler.OnAny(callbacks.OnAny)
	}

	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		state := handler.GetState()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service":          "xplat-sync-cf-receive",
			"updated_at":       state.UpdatedAt,
			"last_event_time":  state.LastEventTime,
			"events_processed": len(state.ProcessedEvents),
		})
	})

	// Main event receiver
	mux.Handle("/", handler)

	log.Printf("sync-cf receive: listening on :%s", port)
	log.Printf("  Health: http://localhost:%s/health", port)
	log.Printf("  Status: http://localhost:%s/status", port)
	log.Printf("  Receive: POST http://localhost:%s/", port)
	log.Printf("")
	log.Printf("Configure Worker's SYNC_ENDPOINT to point here via tunnel")

	return http.ListenAndServe(":"+port, mux)
}

// ReceiveCallbacks holds optional callbacks for receive events
type ReceiveCallbacks struct {
	OnPagesDeploy func(ctx context.Context, event WorkerEvent) error
	OnAlert       func(ctx context.Context, event WorkerEvent) error
	OnLogpush     func(ctx context.Context, event WorkerEvent) error
	OnAny         func(ctx context.Context, event WorkerEvent) error
}

// DefaultLogCallback returns a logging callback for debugging
func DefaultLogCallback() func(ctx context.Context, event WorkerEvent) error {
	return func(ctx context.Context, event WorkerEvent) error {
		log.Printf("EVENT: [%s] %s on %s", event.Type, event.Action, event.Resource)
		if event.Metadata != nil {
			if data, err := json.Marshal(event.Metadata); err == nil {
				log.Printf("  Metadata: %s", string(data))
			}
		}
		return nil
	}
}

// LoadReceiveState loads the current receive state from disk
func LoadReceiveState() (*ReceiverState, error) {
	statePath := filepath.Join(config.XplatCache(), "synccf-receive-state.json")

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ReceiverState{
				ProcessedEvents: make(map[string]ProcessedEvent),
			}, nil
		}
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	var state ReceiverState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}

	return &state, nil
}
