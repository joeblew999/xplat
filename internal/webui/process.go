// Package taskui provides a web-based UI for running Taskfile tasks.
//
// This file implements process-compose integration for the UI.
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProcessInfo holds information about a process from process-compose.
type ProcessInfo struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Status     string `json:"status"`
	IsRunning  bool   `json:"is_running"`
	PID        int    `json:"pid"`
	ExitCode   int    `json:"exit_code"`
	Restarts   int    `json:"restarts"`
	SystemTime string `json:"system_time"`
}

// ProcessState represents the response from /processes endpoint.
type ProcessState struct {
	Data []ProcessInfo `json:"data"`
}

// ProcessComposeClient handles communication with process-compose API.
type ProcessComposeClient struct {
	BaseURL string
	client  *http.Client
}

// NewProcessComposeClient creates a new client for process-compose API.
func NewProcessComposeClient(port int) *ProcessComposeClient {
	return &ProcessComposeClient{
		BaseURL: fmt.Sprintf("http://localhost:%d", port),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IsRunning checks if process-compose is running by hitting the /live endpoint.
func (c *ProcessComposeClient) IsRunning() bool {
	resp, err := c.client.Get(c.BaseURL + "/live")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ListProcesses returns all processes from process-compose.
func (c *ProcessComposeClient) ListProcesses() ([]ProcessInfo, error) {
	resp, err := c.client.Get(c.BaseURL + "/processes")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to process-compose: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("process-compose returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var state ProcessState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return state.Data, nil
}

// StartProcess starts a specific process.
func (c *ProcessComposeClient) StartProcess(name string) error {
	req, err := http.NewRequest("POST", c.BaseURL+"/process/start/"+name, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to start process: %s", string(body))
	}

	return nil
}

// StopProcess stops a specific process.
func (c *ProcessComposeClient) StopProcess(name string) error {
	req, err := http.NewRequest("PATCH", c.BaseURL+"/process/stop/"+name, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to stop process: %s", string(body))
	}

	return nil
}

// RestartProcess restarts a specific process.
func (c *ProcessComposeClient) RestartProcess(name string) error {
	req, err := http.NewRequest("POST", c.BaseURL+"/process/restart/"+name, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to restart process: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to restart process: %s", string(body))
	}

	return nil
}

// GetProcessLogs retrieves logs for a specific process.
func (c *ProcessComposeClient) GetProcessLogs(name string, limit int) (string, error) {
	url := fmt.Sprintf("%s/process/logs/%s/0/%d", c.BaseURL, name, limit)
	resp, err := c.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get logs: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	// Parse the log response - process-compose returns JSON with logs field
	// The logs field contains a JSON array of log lines as a string
	var logResp struct {
		Logs string `json:"logs"`
	}
	if err := json.Unmarshal(body, &logResp); err != nil {
		// If not JSON, return raw body
		return string(body), nil
	}

	// The logs field is itself a JSON array string - parse it
	var logLines []string
	if err := json.Unmarshal([]byte(logResp.Logs), &logLines); err != nil {
		// If not a JSON array, return as-is
		return logResp.Logs, nil
	}

	// Join log lines with newlines
	return strings.Join(logLines, "\n"), nil
}

// getStatusColor returns a color for the process status.
func getStatusColor(status string) string {
	switch status {
	case "Running":
		return "#28a745" // green
	case "Completed":
		return "#6c757d" // gray
	case "Disabled":
		return "#6c757d" // gray
	case "Launching":
		return "#ffc107" // yellow
	case "Pending":
		return "#17a2b8" // cyan
	case "Error", "Failed":
		return "#dc3545" // red
	default:
		return "#6c757d" // gray
	}
}
