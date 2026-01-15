package syncgh

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/go-github/v81/github"
)

// ConfigureGitHubWebhook creates a webhook for a GitHub repo using go-github.
// Requires GITHUB_TOKEN environment variable to be set.
// Uses the same go-github library as the rest of syncgh - no external binaries.
//
// This can be used with any webhook URL, including:
//   - Cloudflare tunnel URLs (xplat sync-cf tunnel)
//   - Any public URL that can receive webhooks
func ConfigureGitHubWebhook(repo, webhookURL, events string) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	// Parse owner/repo
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format, expected owner/repo: %s", repo)
	}
	owner, repoName := parts[0], parts[1]

	// Create authenticated client (same pattern as poller.go)
	client := github.NewClient(nil).WithAuthToken(token)
	ctx := context.Background()

	// Check for existing webhooks with same URL
	hooks, _, err := client.Repositories.ListHooks(ctx, owner, repoName, nil)
	if err != nil {
		log.Printf("Warning: could not check existing webhooks: %v", err)
	} else {
		for _, hook := range hooks {
			if hook.Config != nil {
				if url := hook.Config.GetURL(); url == webhookURL {
					log.Printf("Webhook already exists for this URL")
					return nil
				}
			}
		}
	}

	// Create webhook
	eventList := strings.Split(events, ",")
	hook := &github.Hook{
		Name:   github.String("web"),
		Active: github.Bool(true),
		Events: eventList,
		Config: &github.HookConfig{
			URL:         github.String(webhookURL),
			ContentType: github.String("json"),
			InsecureSSL: github.String("0"),
		},
	}

	_, _, err = client.Repositories.CreateHook(ctx, owner, repoName, hook)
	if err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	log.Printf("Webhook created for %s -> %s", repo, webhookURL)
	return nil
}

// ListWebhooks lists all webhooks for a GitHub repo.
func ListWebhooks(repo string) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format, expected owner/repo: %s", repo)
	}
	owner, repoName := parts[0], parts[1]

	client := github.NewClient(nil).WithAuthToken(token)
	ctx := context.Background()

	hooks, _, err := client.Repositories.ListHooks(ctx, owner, repoName, nil)
	if err != nil {
		return fmt.Errorf("failed to list webhooks: %w", err)
	}

	if len(hooks) == 0 {
		log.Printf("No webhooks configured for %s", repo)
		return nil
	}

	log.Printf("Webhooks for %s:", repo)
	for _, hook := range hooks {
		url := ""
		if hook.Config != nil {
			url = hook.Config.GetURL()
		}
		active := "inactive"
		if hook.GetActive() {
			active = "active"
		}
		log.Printf("  [%d] %s (%s) - events: %v", hook.GetID(), url, active, hook.Events)
	}

	return nil
}

// DeleteWebhook deletes a webhook by ID.
func DeleteWebhook(repo string, hookID int64) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format, expected owner/repo: %s", repo)
	}
	owner, repoName := parts[0], parts[1]

	client := github.NewClient(nil).WithAuthToken(token)
	ctx := context.Background()

	_, err := client.Repositories.DeleteHook(ctx, owner, repoName, hookID)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	log.Printf("Webhook %d deleted from %s", hookID, repo)
	return nil
}
