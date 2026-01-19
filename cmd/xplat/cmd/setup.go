// Package cmd provides CLI commands for xplat.
//
// setup.go - Environment configuration wizard commands
package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

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

	// Try to open browser
	go func() {
		// Small delay to let server start
		url := "https://localhost/admin/"
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

func runEnvCheck(cmd *cobra.Command, args []string) error {
	fmt.Println("Checking environment configuration...")
	fmt.Println()

	// TODO: Implement validation
	// 1. Load .env file
	// 2. Load manifest to get required env vars
	// 3. Check each required var exists
	// 4. Optionally validate API tokens

	fmt.Println("(Not yet implemented - see ADR-017)")
	return nil
}

func runEnvStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Environment configuration status:")
	fmt.Println()

	// TODO: Implement status display
	// 1. Load .env file
	// 2. Load manifest to get required/optional vars
	// 3. Display table of configured vs missing

	fmt.Println("(Not yet implemented - see ADR-017)")
	return nil
}
