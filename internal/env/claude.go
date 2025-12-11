package env

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClaudeMessageRequest represents a minimal Claude API request
type ClaudeMessageRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []ClaudeMessage `json:"messages"`
}

// ClaudeMessage represents a message in the request
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeResponse represents the Claude API response
type ClaudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ClaudeErrorResponse represents an error response from Claude API
type ClaudeErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// ValidateClaudeAPIKey validates a Claude API key
func ValidateClaudeAPIKey(apiKey string) error {
	if apiKey == "" || apiKey == PlaceholderKey {
		return fmt.Errorf("no API key to validate")
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// Make a minimal API request to test the key
	reqBody := ClaudeMessageRequest{
		Model:     "claude-3-haiku-20240307",
		MaxTokens: 10,
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: "test",
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", AnthropicAPIMessagesURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		var errResp ClaudeErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return fmt.Errorf("API request failed with status %d", resp.StatusCode)
		}

		return fmt.Errorf("%s: %s", errResp.Error.Type, errResp.Error.Message)
	}

	// Parse success response
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}
