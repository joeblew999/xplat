// Package cmd provides CLI commands for xplat.
//
// setup.go - Environment configuration wizard commands
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/env"
	"github.com/joeblew999/xplat/internal/env/web"
	"github.com/spf13/cobra"
)

// SetupCmd is the parent command for environment configuration.
var SetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Environment configuration wizard",
	Long: `Guided setup wizard for configuring external services.

The setup wizard provides a web UI for configuring:
- Cloudflare (API tokens, Pages, Workers, Tunnels)
- Claude AI (API keys for translation)
- Other external service integrations

Configuration is saved to .env file (git-ignored).

Examples:
  xplat setup wizard       # Launch web-based setup wizard
  xplat setup wizard --mock  # Launch in mock mode (no real API calls)
  xplat setup check        # Validate current configuration
  xplat setup status       # Show what's configured vs missing`,
}

var envWizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Launch web-based environment setup wizard",
	Long: `Start a local web server with a guided wizard for configuring
external services like Cloudflare and Claude AI.

The wizard will:
1. Open your browser to the setup UI
2. Guide you through each configuration step
3. Validate credentials before saving
4. Write configuration to .env file

Press Ctrl+C to stop the wizard server.`,
	RunE: runEnvWizard,
}

var envCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate current environment configuration",
	Long: `Check that all required environment variables are set and valid.

Validates:
- Required variables from xplat.yaml manifest
- API token validity (where possible)
- Configuration completeness`,
	RunE: runEnvCheck,
}

var envStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show environment configuration status",
	Long: `Display which environment variables are configured and which are missing.

Shows:
- Configured variables (from .env)
- Missing required variables
- Optional variables with defaults`,
	RunE: runEnvStatus,
}

var envMockMode bool

func init() {
	envWizardCmd.Flags().BoolVar(&envMockMode, "mock", false, "Run in mock mode (no real API validation)")

	SetupCmd.AddCommand(envWizardCmd)
	SetupCmd.AddCommand(envCheckCmd)
	SetupCmd.AddCommand(envStatusCmd)
}

func runEnvWizard(cmd *cobra.Command, args []string) error {
	fmt.Println("Starting environment setup wizard...")
	fmt.Println()

	// Try to open browser after a small delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		url := fmt.Sprintf("https://localhost:%d/admin/", config.DefaultUIPortInt)
		var err error
		switch runtime.GOOS {
		case "darwin":
			err = exec.Command("open", url).Start()
		case "linux":
			err = exec.Command("xdg-open", url).Start()
		case "windows":
			err = exec.Command("cmd", "/c", "start", url).Start()
		}
		if err != nil {
			fmt.Printf("Could not open browser automatically. Please visit: %s\n", url)
		}
	}()

	if envMockMode {
		return web.ServeSetupGUIMock()
	}
	return web.ServeSetupGUI()
}

var envCheckDeep bool

func init() {
	envCheckCmd.Flags().BoolVar(&envCheckDeep, "deep", false, "Perform deep validation (makes API calls to verify tokens)")
}

func runEnvCheck(cmd *cobra.Command, args []string) error {
	// Check if .env exists
	if !env.EnvExists() {
		fmt.Println("No .env file found.")
		fmt.Println()
		fmt.Println("Run 'xplat setup wizard' to configure your environment.")
		os.Exit(1)
	}

	// Load config
	cfg, err := env.LoadEnv()
	if err != nil {
		return fmt.Errorf("failed to load .env: %w", err)
	}

	svc := env.NewService(envMockMode)

	var results map[string]env.ValidationResult
	if envCheckDeep {
		fmt.Println("Validating environment configuration (deep)...")
		fmt.Println()
		results = svc.ValidateConfigDeep(cfg)
	} else {
		fmt.Println("Validating environment configuration (fast)...")
		fmt.Println("(Use --deep for API token verification)")
		fmt.Println()
		results = svc.ValidateConfigFast(cfg)
	}

	// Display results
	hasErrors := false
	for _, field := range env.GetAllFieldsInOrder() {
		result, ok := results[field.Key]
		if !ok {
			continue
		}

		if result.Skipped {
			fmt.Printf("  ⚪ %s (skipped)\n", field.DisplayName)
		} else if result.Valid {
			fmt.Printf("  ✓ %s\n", field.DisplayName)
		} else {
			hasErrors = true
			errMsg := "invalid"
			if result.Error != nil {
				errMsg = result.Error.Error()
			}
			fmt.Printf("  ✗ %s: %s\n", field.DisplayName, errMsg)
		}
	}

	fmt.Println()
	if hasErrors {
		fmt.Println("Some configuration is invalid. Run 'xplat setup wizard' to fix.")
		os.Exit(1)
	}
	fmt.Println("All configuration is valid.")
	return nil
}

func runEnvStatus(cmd *cobra.Command, args []string) error {
	// Check if .env exists
	if !env.EnvExists() {
		fmt.Println("No .env file found.")
		fmt.Println()
		fmt.Println("Run 'xplat setup wizard' to configure your environment.")
		return nil
	}

	// Load config
	cfg, err := env.LoadEnv()
	if err != nil {
		return fmt.Errorf("failed to load .env: %w", err)
	}

	envPath, _ := env.GetEnvPath()
	fmt.Printf("Configuration file: %s\n", envPath)
	fmt.Println()

	// Count configured vs missing
	configured := 0
	missing := 0
	optional := 0

	fmt.Println("Environment variables:")
	fmt.Println()

	for _, field := range env.GetAllFieldsInOrder() {
		value := cfg.Get(field.Key)
		isSet := value != "" && !env.IsPlaceholder(value)

		status := "✗"
		statusText := "missing"

		if isSet {
			status = "✓"
			// Mask sensitive values
			if field.Key == env.KeyCloudflareAPIToken || field.Key == env.KeyClaudeAPIKey {
				if len(value) > 8 {
					statusText = value[:4] + "..." + value[len(value)-4:]
				} else {
					statusText = "****"
				}
			} else {
				statusText = value
			}
			configured++
		} else if !field.Validate {
			status = "⚪"
			statusText = "(optional)"
			optional++
		} else {
			missing++
		}

		// Show required marker
		reqMarker := ""
		if field.Validate {
			reqMarker = "*"
		}

		fmt.Printf("  %s %s%s: %s\n", status, field.DisplayName, reqMarker, statusText)
	}

	fmt.Println()
	fmt.Printf("Summary: %d configured, %d missing, %d optional\n", configured, missing, optional)

	if missing > 0 {
		fmt.Println()
		fmt.Println("Run 'xplat setup wizard' to configure missing variables.")
	}

	return nil
}
