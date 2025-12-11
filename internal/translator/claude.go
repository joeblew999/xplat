package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	claudeAPIURL = "https://api.anthropic.com/v1/messages"
	claudeModel  = "claude-sonnet-4-20250514"
	maxTokens    = 4096
)

// ClaudeClient handles communication with Claude API or CLI
type ClaudeClient struct {
	apiKey     string
	httpClient *http.Client
	useCLI     bool // true if using claude CLI instead of API
}

// ClaudeRequest represents a request to Claude API
type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []ClaudeMessage `json:"messages"`
}

// ClaudeMessage represents a message in the conversation
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeResponse represents a response from Claude API
type ClaudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// NewClaudeClient creates a new Claude client (API or CLI)
// If apiKey is empty, uses the claude CLI (web auth)
func NewClaudeClient(apiKey string) (*ClaudeClient, error) {
	if apiKey == "" {
		return &ClaudeClient{
			useCLI: true,
		}, nil
	}

	return &ClaudeClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		useCLI: false,
	}, nil
}

// Translate translates text to the target language
func (c *ClaudeClient) Translate(text, targetLang, targetLangName string) (string, error) {
	prompt := fmt.Sprintf("Translate the following Hugo markdown content from English to %s (%s).\n\n"+
		"IMPORTANT INSTRUCTIONS:\n"+
		"1. Translate ONLY the readable text content\n"+
		"2. DO NOT translate:\n"+
		"   - Hugo shortcodes (e.g., {{< button >}}, {{< notice >}})\n"+
		"   - Code blocks (content between triple backtick markers)\n"+
		"   - URLs and links\n"+
		"   - HTML tags\n"+
		"   - YAML/TOML front matter field names\n"+
		"3. Preserve all markdown formatting (headers, lists, bold, italic, etc.)\n"+
		"4. Maintain the same structure and paragraph breaks\n"+
		"5. Keep the tone and style appropriate for the content type\n\n"+
		"Content to translate:\n\n%s\n\n"+
		"Please provide ONLY the translated text, with no explanations or additional commentary.",
		targetLangName, targetLang, text)

	if c.useCLI {
		return c.callCLI(prompt)
	}
	return c.callAPI(prompt)
}

// TranslateI18n translates i18n data
func (c *ClaudeClient) TranslateI18n(data map[string]string, targetLang, targetLangName string) (map[string]string, error) {
	// Convert map to JSON for easier handling
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal i18n data: %w", err)
	}

	prompt := fmt.Sprintf("Translate the following i18n (internationalization) strings from English to %s (%s).\n\n"+
		"IMPORTANT INSTRUCTIONS:\n"+
		"1. Translate ONLY the values (the text after the colon)\n"+
		"2. DO NOT translate the keys (the text before the colon)\n"+
		"3. Preserve the JSON structure exactly\n"+
		"4. Keep placeholders like {{.Name}} unchanged\n"+
		"5. Maintain appropriate context for UI strings\n\n"+
		"i18n data to translate:\n\n%s\n\n"+
		"Please provide ONLY the translated JSON, with no explanations or additional commentary.",
		targetLangName, targetLang, string(jsonData))

	var translated string
	if c.useCLI {
		translated, err = c.callCLI(prompt)
	} else {
		translated, err = c.callAPI(prompt)
	}
	if err != nil {
		return nil, err
	}

	// Parse the translated JSON back to map
	var result map[string]string
	if err := json.Unmarshal([]byte(translated), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal translated i18n: %w", err)
	}

	return result, nil
}

// callAPI makes a request to Claude API
func (c *ClaudeClient) callAPI(prompt string) (string, error) {
	req := ClaudeRequest{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", claudeAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return claudeResp.Content[0].Text, nil
}

// callCLI uses the claude CLI for translation (web auth, no API key needed)
func (c *ClaudeClient) callCLI(prompt string) (string, error) {
	// Use claude CLI with -p flag for prompt mode (non-interactive)
	cmd := exec.Command("claude", "-p", prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude CLI failed: %w\nstderr: %s", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("claude CLI returned empty response")
	}

	return output, nil
}
