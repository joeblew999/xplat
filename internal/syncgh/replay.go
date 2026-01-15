// Package syncgh provides GitHub sync operations.
//
// This file implements webhook delivery replay from GitHub API.
// Pattern adopted from github.com/chmouel/gosmee for compatibility.
package syncgh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v81/github"
)

// ReplayConfig holds configuration for webhook replay.
type ReplayConfig struct {
	// Owner is the GitHub owner (user or org)
	Owner string

	// Repo is the repository name (empty for org-level hooks)
	Repo string

	// HookID is the webhook ID to replay deliveries from
	HookID int64

	// TargetURL is where to forward replayed webhooks
	TargetURL string

	// Since replays only deliveries after this time
	Since time.Time

	// SaveDir saves payloads to disk (optional)
	SaveDir string

	// IgnoreEvents skips these event types
	IgnoreEvents []string

	// Continuous keeps polling for new deliveries
	Continuous bool

	// Token is the GitHub token for API access
	Token string
}

// ReplayResult contains information about a replayed delivery.
type ReplayResult struct {
	DeliveryID  int64
	GUID        string
	Event       string
	DeliveredAt time.Time
	StatusCode  int
	Error       error
}

// Replayer handles webhook delivery replay from GitHub API.
type Replayer struct {
	config ReplayConfig
	client *github.Client
}

// NewReplayer creates a new webhook replayer.
func NewReplayer(config ReplayConfig) *Replayer {
	client := github.NewClient(nil)
	if config.Token != "" {
		client = client.WithAuthToken(config.Token)
	}

	return &Replayer{
		config: config,
		client: client,
	}
}

// ListHooks lists webhooks for a repo or org.
func (r *Replayer) ListHooks(ctx context.Context) ([]*github.Hook, error) {
	var hooks []*github.Hook
	var err error

	if r.config.Repo != "" {
		hooks, _, err = r.client.Repositories.ListHooks(ctx, r.config.Owner, r.config.Repo, nil)
	} else {
		hooks, _, err = r.client.Organizations.ListHooks(ctx, r.config.Owner, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list hooks: %w", err)
	}

	return hooks, nil
}

// ListDeliveries lists recent webhook deliveries for a hook.
func (r *Replayer) ListDeliveries(ctx context.Context, hookID int64) ([]*github.HookDelivery, error) {
	opt := &github.ListCursorOptions{PerPage: 50}

	var deliveries []*github.HookDelivery
	var err error

	if r.config.Repo != "" {
		deliveries, _, err = r.client.Repositories.ListHookDeliveries(ctx, r.config.Owner, r.config.Repo, hookID, opt)
	} else {
		deliveries, _, err = r.client.Organizations.ListHookDeliveries(ctx, r.config.Owner, hookID, opt)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries: %w", err)
	}

	return deliveries, nil
}

// GetDelivery gets the full details of a webhook delivery.
func (r *Replayer) GetDelivery(ctx context.Context, hookID, deliveryID int64) (*github.HookDelivery, error) {
	var delivery *github.HookDelivery
	var err error

	if r.config.Repo != "" {
		delivery, _, err = r.client.Repositories.GetHookDelivery(ctx, r.config.Owner, r.config.Repo, hookID, deliveryID)
	} else {
		delivery, _, err = r.client.Organizations.GetHookDelivery(ctx, r.config.Owner, hookID, deliveryID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get delivery: %w", err)
	}

	return delivery, nil
}

// Replay replays webhook deliveries to the target URL.
// If Continuous is true, it keeps polling for new deliveries.
func (r *Replayer) Replay(ctx context.Context) error {
	if r.config.HookID == 0 {
		return fmt.Errorf("hook ID is required")
	}
	if r.config.TargetURL == "" {
		return fmt.Errorf("target URL is required")
	}

	sinceTime := r.config.Since
	if sinceTime.IsZero() {
		// Default to now (only new deliveries in continuous mode)
		sinceTime = time.Now()
	}

	log.Printf("Replay: Starting replay for hook %d", r.config.HookID)
	if r.config.Repo != "" {
		log.Printf("Replay: Repository: %s/%s", r.config.Owner, r.config.Repo)
	} else {
		log.Printf("Replay: Organization: %s", r.config.Owner)
	}
	log.Printf("Replay: Target URL: %s", r.config.TargetURL)
	log.Printf("Replay: Since: %s", sinceTime.Format(time.RFC3339))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		deliveries, err := r.ListDeliveries(ctx, r.config.HookID)
		if err != nil {
			log.Printf("Replay: Error listing deliveries: %v", err)
			if !r.config.Continuous {
				return err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		// Filter and reverse deliveries (oldest first)
		filtered := r.filterDeliveries(deliveries, sinceTime)

		for _, hd := range filtered {
			result := r.replayDelivery(ctx, hd)
			if result.Error != nil {
				log.Printf("Replay: Error replaying delivery %d: %v", result.DeliveryID, result.Error)
			} else {
				log.Printf("Replay: Replayed %s [%s] -> %d", result.Event, result.GUID, result.StatusCode)
			}

			// Update since time
			if hd.DeliveredAt != nil {
				sinceTime = hd.DeliveredAt.Time.Add(1 * time.Second)
			}
		}

		if !r.config.Continuous {
			break
		}

		time.Sleep(5 * time.Second)
	}

	return nil
}

// filterDeliveries filters and reverses deliveries (oldest first, after sinceTime).
func (r *Replayer) filterDeliveries(deliveries []*github.HookDelivery, sinceTime time.Time) []*github.HookDelivery {
	var filtered []*github.HookDelivery

	for _, d := range deliveries {
		if d.DeliveredAt == nil {
			continue
		}
		if d.DeliveredAt.Time.Before(sinceTime) {
			break
		}
		filtered = append(filtered, d)
	}

	// Reverse to get oldest first
	for i := len(filtered)/2 - 1; i >= 0; i-- {
		opp := len(filtered) - 1 - i
		filtered[i], filtered[opp] = filtered[opp], filtered[i]
	}

	return filtered
}

// replayDelivery replays a single webhook delivery.
func (r *Replayer) replayDelivery(ctx context.Context, hd *github.HookDelivery) ReplayResult {
	result := ReplayResult{
		DeliveryID:  hd.GetID(),
		GUID:        hd.GetGUID(),
		Event:       hd.GetEvent(),
		DeliveredAt: hd.GetDeliveredAt().Time,
	}

	// Check if event should be ignored
	if len(r.config.IgnoreEvents) > 0 {
		for _, ignore := range r.config.IgnoreEvents {
			if strings.EqualFold(hd.GetEvent(), ignore) {
				log.Printf("Replay: Skipping ignored event: %s", hd.GetEvent())
				return result
			}
		}
	}

	// Get full delivery details
	delivery, err := r.GetDelivery(ctx, r.config.HookID, hd.GetID())
	if err != nil {
		result.Error = err
		return result
	}

	if delivery.Request == nil {
		result.Error = fmt.Errorf("delivery has no request data")
		return result
	}

	// Build and send request
	payload := []byte(delivery.Request.GetRawPayload())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.TargetURL, bytes.NewReader(payload))
	if err != nil {
		result.Error = fmt.Errorf("failed to create request: %w", err)
		return result
	}

	// Copy headers from original request
	for k, v := range delivery.Request.Headers {
		req.Header.Set(k, v)
	}

	// Ensure content-type is set
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("failed to send request: %w", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Errorf("target returned error: %d %s", resp.StatusCode, string(body))
	}

	// Save payload if configured
	if r.config.SaveDir != "" {
		if err := r.savePayload(delivery, payload); err != nil {
			log.Printf("Replay: Failed to save payload: %v", err)
		}
	}

	return result
}

// savePayload saves the webhook payload to disk.
func (r *Replayer) savePayload(delivery *github.HookDelivery, payload []byte) error {
	if err := os.MkdirAll(r.config.SaveDir, 0o755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	ts := delivery.GetDeliveredAt().Time.Format("2006-01-02T15.04.05.000")
	event := delivery.GetEvent()
	if event == "" {
		event = "unknown"
	}

	baseName := fmt.Sprintf("%s-%s", event, ts)

	// Save JSON payload
	jsonPath := filepath.Join(r.config.SaveDir, baseName+".json")
	if err := os.WriteFile(jsonPath, payload, 0o644); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	// Save replay script
	shPath := filepath.Join(r.config.SaveDir, baseName+".sh")
	script := r.buildReplayScript(delivery, baseName)
	if err := os.WriteFile(shPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("failed to write shell script: %w", err)
	}

	log.Printf("Replay: Saved payload to %s", jsonPath)
	return nil
}

// buildReplayScript creates a curl command to replay the webhook.
func (r *Replayer) buildReplayScript(delivery *github.HookDelivery, baseName string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# Replay webhook event\n")
	sb.WriteString("# Generated by xplat sync-gh replay\n\n")

	sb.WriteString("TARGET_URL=\"${1:-")
	sb.WriteString(r.config.TargetURL)
	sb.WriteString("}\"\n\n")

	sb.WriteString("curl -X POST \"$TARGET_URL\" \\\n")

	// Add headers
	if delivery.Request != nil {
		for k, v := range delivery.Request.Headers {
			sb.WriteString(fmt.Sprintf("  -H '%s: %s' \\\n", k, v))
		}
	}

	sb.WriteString(fmt.Sprintf("  -d @\"$(dirname \"$0\")/%s.json\"\n", baseName))

	return sb.String()
}

// PrintHooks prints hooks in a formatted table.
func PrintHooks(hooks []*github.Hook) {
	fmt.Printf("%-12s %-20s %s\n", "ID", "Name", "URL")
	fmt.Println(strings.Repeat("-", 60))

	for _, h := range hooks {
		url := ""
		if h.Config != nil {
			url = h.Config.GetURL()
		}
		fmt.Printf("%-12d %-20s %s\n", h.GetID(), h.GetName(), url)
	}
}

// PrintDeliveries prints deliveries in a formatted table.
func PrintDeliveries(deliveries []*github.HookDelivery) {
	fmt.Printf("%-12s %-15s %-8s %s\n", "ID", "Event", "Status", "Delivered At")
	fmt.Println(strings.Repeat("-", 70))

	for _, d := range deliveries {
		status := "unknown"
		if d.StatusCode != nil {
			if *d.StatusCode >= 200 && *d.StatusCode < 300 {
				status = "success"
			} else {
				status = fmt.Sprintf("%d", *d.StatusCode)
			}
		}

		deliveredAt := ""
		if d.DeliveredAt != nil {
			deliveredAt = d.DeliveredAt.Time.Format(time.RFC3339)
		}

		fmt.Printf("%-12d %-15s %-8s %s\n", d.GetID(), d.GetEvent(), status, deliveredAt)
	}
}

// RunReplayListHooks lists hooks for a repo or org.
func RunReplayListHooks(owner, repo, token string) error {
	replayer := NewReplayer(ReplayConfig{
		Owner: owner,
		Repo:  repo,
		Token: token,
	})

	hooks, err := replayer.ListHooks(context.Background())
	if err != nil {
		return err
	}

	if len(hooks) == 0 {
		fmt.Println("No webhooks found.")
		return nil
	}

	PrintHooks(hooks)
	return nil
}

// RunReplayListDeliveries lists deliveries for a hook.
func RunReplayListDeliveries(owner, repo string, hookID int64, token string) error {
	replayer := NewReplayer(ReplayConfig{
		Owner:  owner,
		Repo:   repo,
		HookID: hookID,
		Token:  token,
	})

	deliveries, err := replayer.ListDeliveries(context.Background(), hookID)
	if err != nil {
		return err
	}

	if len(deliveries) == 0 {
		fmt.Println("No deliveries found.")
		return nil
	}

	PrintDeliveries(deliveries)
	return nil
}

// RunReplay replays webhook deliveries.
func RunReplay(config ReplayConfig) error {
	replayer := NewReplayer(config)
	return replayer.Replay(context.Background())
}
