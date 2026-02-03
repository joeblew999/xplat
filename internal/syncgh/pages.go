package syncgh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// EnablePages enables GitHub Pages for a repository using the workflow build type.
// It uses GITHUB_TOKEN environment variable for authentication.
// Returns nil if pages are already enabled or successfully enabled.
func EnablePages(owner, repo string) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN not set")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pages", owner, repo)
	body := map[string]string{"build_type": "workflow"}
	jsonBody, _ := json.Marshal(body)

	// Try POST first (create new)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// 201 = created, 409 = already exists
	if resp.StatusCode == 201 {
		return nil
	}

	if resp.StatusCode == 409 {
		// Already exists, try PUT to update
		req, _ = http.NewRequest("PUT", url, bytes.NewBuffer(jsonBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == 204 || resp.StatusCode == 200 {
			return nil
		}
	}

	// 404 might mean repo doesn't exist or no permission
	if resp.StatusCode == 404 {
		return fmt.Errorf("repo not found or insufficient permissions")
	}

	return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// GetRepoFromRemote extracts owner/repo from a git remote URL.
// Supports: https://github.com/owner/repo.git and git@github.com:owner/repo.git
func GetRepoFromRemote(remoteURL string) (owner, repo string, err error) {
	remoteURL = strings.TrimSuffix(remoteURL, ".git")

	if strings.HasPrefix(remoteURL, "https://github.com/") {
		parts := strings.Split(strings.TrimPrefix(remoteURL, "https://github.com/"), "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	}

	if strings.HasPrefix(remoteURL, "git@github.com:") {
		parts := strings.Split(strings.TrimPrefix(remoteURL, "git@github.com:"), "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("unsupported remote URL format: %s", remoteURL)
}
