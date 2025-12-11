package env

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Global Hugo server management
var (
	hugoServerCmd  *exec.Cmd
	hugoServerMux  sync.Mutex
	hugoServerPort = 1313 // Default Hugo server port
)

// StartHugoServer starts a simple HTTP server for local preview of the built site
// Binds to 0.0.0.0 for LAN access (mobile testing)
// Note: Uses HTTP instead of HTTPS to avoid certificate trust issues on mobile devices
func StartHugoServer(mockMode bool) CommandOutput {
	hugoServerMux.Lock()
	defer hugoServerMux.Unlock()

	// Stop any existing server first
	if hugoServerCmd != nil {
		stopHugoServerInternal()
	}

	// Detect LAN IP address for mobile testing (empty string if not available)
	lanIP := GetLocalIPOrFallback()

	if mockMode {
		localURL := fmt.Sprintf("http://localhost:%d", hugoServerPort)
		lanURL := ""
		if lanIP != "" {
			lanURL = fmt.Sprintf("http://%s:%d", lanIP, hugoServerPort)
		}
		output := fmt.Sprintf("Starting preview server (mock mode)...\n  Local: %s\n  LAN:   %s", localURL, lanURL)
		return CommandOutput{
			Output:   output,
			Error:    nil,
			LocalURL: localURL,
			LANURL:   lanURL,
		}
	}

	// Start Hugo server with HTTP (no HTTPS certificates needed)
	// Use development environment which enables relativeURLs for multi-hostname support
	// IMPORTANT: --environment must come BEFORE server subcommand for Hugo to load the config
	// NOTE: Do NOT set --baseURL flag - let Hugo use config/development/config.toml baseURL="/"
	//       This ensures the <base> tag uses relative URLs instead of hardcoded production URL
	hugoServerCmd = exec.Command("hugo",
		"--environment", "development", // Loads config/development/config.toml with baseURL="/" and relativeURLs=true
		"server",
		"--disableLiveReload",
		"--port", fmt.Sprintf("%d", hugoServerPort),
		"--bind", "0.0.0.0", // Bind to all interfaces for LAN access
	)

	// Start the server in background
	if err := hugoServerCmd.Start(); err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to start preview server: %w", err),
		}
	}

	// Build URLs for display
	localURL := fmt.Sprintf("http://localhost:%d", hugoServerPort)
	lanURL := ""
	if lanIP != "" {
		lanURL = fmt.Sprintf("http://%s:%d", lanIP, hugoServerPort)
	}

	output := fmt.Sprintf("Preview server started\n  Local: %s\n  LAN:   %s", localURL, lanURL)

	return CommandOutput{
		Output:   output,
		Error:    nil,
		LocalURL: localURL,
		LANURL:   lanURL,
	}
}

// stopHugoServerInternal stops the Hugo server without acquiring the mutex.
// This internal function should only be called when the caller already holds hugoServerMux.
func stopHugoServerInternal() CommandOutput {
	if hugoServerCmd == nil {
		return CommandOutput{
			Output: "No Hugo server is running",
			Error:  nil,
		}
	}

	// Kill the server process
	if err := hugoServerCmd.Process.Kill(); err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to stop Hugo server: %w", err),
		}
	}

	hugoServerCmd = nil

	return CommandOutput{
		Output: "Hugo preview server stopped",
		Error:  nil,
	}
}

// StopHugoServer stops the running Hugo server
func StopHugoServer() CommandOutput {
	hugoServerMux.Lock()
	defer hugoServerMux.Unlock()

	// Unregister Hugo service from Caddy (shutdown event)
	if err := UnregisterService("hugo"); err != nil {
		fmt.Printf("Warning: Failed to unregister Hugo from Caddy: %v\n", err)
	}

	return stopHugoServerInternal()
}

// BuildHugoSite runs `hugo --gc --minify` and returns streaming output
// Also starts Caddy for HTTPS and a local HTTP preview server with LAN access
func BuildHugoSite(mockMode bool) CommandOutput {
	// Ensure Caddy is running for HTTPS support
	if err := EnsureCaddyRunning(); err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to ensure Caddy is running: %w", err),
		}
	}

	// Register Hugo service with Caddy (event-based pattern)
	regResult, err := RegisterService(ServiceConfig{
		Name:        "hugo",
		Port:        1313,
		PathPattern: "",  // Root path (catch-all)
		Priority:    0,   // Lower priority than Via GUI
		HealthPath:  "/", // Health check endpoint (Hugo homepage)
	})
	if err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to register Hugo with Caddy: %w", err),
		}
	}

	// Use the URLs provided by Caddy registration
	localURL := regResult.FullLocalURL
	lanURL := regResult.FullLANURL

	if mockMode {
		output := fmt.Sprintf("Building Hugo site (mock mode)...\nBuild complete! (mock)\n\nStarting preview server...\nPreview server running\n  Local: %s\n  LAN:   %s", localURL, lanURL)
		return CommandOutput{
			Output:   output,
			Error:    nil,
			LocalURL: localURL,
			LANURL:   lanURL,
		}
	}

	// Build the site using development environment (same as server)
	// This ensures built files use baseURL="/" and relativeURLs=true from config/development/config.toml
	buildResult := runCommand("hugo", "--environment", "development", "--gc", "--minify")
	if buildResult.Error != nil {
		return buildResult
	}

	// Start preview server with HTTP and LAN access (Caddy provides HTTPS wrapper)
	serverResult := StartHugoServer(mockMode)
	if serverResult.Error != nil {
		// Build succeeded but server failed - return build output with warning
		return CommandOutput{
			Output:   buildResult.Output + "\n\nWarning: Failed to start preview server: " + serverResult.Error.Error(),
			Error:    nil,
			LocalURL: "",
			LANURL:   "",
		}
	}

	// Combine build and server outputs with HTTPS URLs from registration
	httpsOutput := fmt.Sprintf("Preview server started with HTTPS\n  Local: %s\n  LAN:   %s", localURL, lanURL)
	return CommandOutput{
		Output:   buildResult.Output + "\n\n" + httpsOutput,
		Error:    nil,
		LocalURL: localURL,
		LANURL:   lanURL,
	}
}

// GetHugoVersion returns the Hugo version and binary location
func GetHugoVersion() (version string, binaryPath string, err error) {
	// Try to find hugo in PATH
	binaryPath, err = exec.LookPath("hugo")
	if err != nil {
		return "", "", fmt.Errorf("hugo binary not found in PATH\n" +
			"Install: https://gohugo.io/installation/")
	}

	// Get version
	cmd := exec.Command("hugo", "version")
	output, err := cmd.Output()
	if err != nil {
		return "", binaryPath, fmt.Errorf("failed to get hugo version: %w", err)
	}

	version = strings.TrimSpace(string(output))
	return version, binaryPath, nil
}

// PrintHugoVersion prints Hugo version and binary location
func PrintHugoVersion() {
	version, binaryPath, err := GetHugoVersion()
	if err != nil {
		fmt.Printf("Hugo: %v\n", err)
		return
	}
	fmt.Printf("Hugo:\n")
	fmt.Printf("  Binary: %s\n", binaryPath)
	fmt.Printf("  Version: %s\n", version)
}
