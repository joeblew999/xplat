// Package cmd provides CLI commands for xplat.
//
// internal.go - Commands for xplat developers only
//
// The internal: namespace contains commands for xplat development:
// - internal:docs - Generate xplat's own documentation
// - internal:dev  - Developer utilities (install, etc.)
// - internal:gen  - Generate xplat's own CI/install scripts from config
//
// Users of plat-* repos should use 'xplat gen' instead.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/templates"
	"github.com/spf13/cobra"
)

// InternalCmd is the parent for all internal xplat developer commands.
var InternalCmd = &cobra.Command{
	Use:   "internal",
	Short: "xplat developer commands (not for end users)",
	Long: `Commands for xplat developers only.

If you're working in a plat-* repo, use 'xplat gen' instead.

Subcommands:
  internal:docs    Generate xplat's own documentation
  internal:dev     Developer utilities (install, build, etc.)
  internal:gen     Generate xplat's own CI/install scripts`,
}

// === internal:gen - Self-generation commands ===

var internalGenCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate xplat's own CI/install scripts from config",
	Long: `Generate xplat's install.sh and CI action from config.go.

This ensures all install paths are consistent with the source of truth
defined in internal/config/config.go.

Templates are stored in internal/templates/*.tmpl

Examples:
  xplat internal gen all       # Generate all files
  xplat internal gen install   # Generate install.sh only
  xplat internal gen action    # Generate .github/actions/setup/action.yml only`,
}

var internalGenAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Generate all self-managed files",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runInternalGenInstall(cmd, args); err != nil {
			return err
		}
		return runInternalGenAction(cmd, args)
	},
}

var internalGenInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Generate install.sh from config",
	RunE:  runInternalGenInstall,
}

var internalGenActionCmd = &cobra.Command{
	Use:   "action",
	Short: "Generate .github/actions/setup/action.yml from config",
	RunE:  runInternalGenAction,
}

var internalGenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available templates",
	RunE:  runInternalGenList,
}

var internalGenOutputDir string

func init() {
	// Add subcommands to internal:gen
	internalGenCmd.AddCommand(internalGenAllCmd)
	internalGenCmd.AddCommand(internalGenInstallCmd)
	internalGenCmd.AddCommand(internalGenActionCmd)
	internalGenCmd.AddCommand(internalGenListCmd)
	internalGenCmd.PersistentFlags().StringVarP(&internalGenOutputDir, "output", "o", ".", "Output directory")

	// Add all subcommands to internal:
	InternalCmd.AddCommand(internalGenCmd)
}

// getInstallData creates InstallData from config.go values.
func getInstallData() templates.InstallData {
	// Get the install directory from canonical path
	canonicalBin := config.XplatCanonicalBin()
	unixDir := filepath.Dir(canonicalBin)
	// Convert to shell variable path for the template
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(unixDir, home) {
		unixDir = "$HOME" + unixDir[len(home):]
	}

	return templates.InstallData{
		UnixInstallDir:    unixDir,
		WindowsInstallDir: "$LOCALAPPDATA/xplat",
		BinaryName:        "xplat",
		Repo:              config.XplatRepo,
		TagPrefix:         config.XplatTagPrefix,
		ChecksumFile:      config.XplatChecksumFile,
		StaleLocations:    config.XplatStaleLocations(),
	}
}

func runInternalGenInstall(cmd *cobra.Command, args []string) error {
	content, err := templates.RenderInternal("install.sh.tmpl", getInstallData())
	if err != nil {
		return fmt.Errorf("failed to render install.sh: %w", err)
	}

	outPath := filepath.Join(internalGenOutputDir, "install.sh")
	if err := os.WriteFile(outPath, content, 0755); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

func runInternalGenAction(cmd *cobra.Command, args []string) error {
	content, err := templates.RenderInternal("action.yml.tmpl", getInstallData())
	if err != nil {
		return fmt.Errorf("failed to render action.yml: %w", err)
	}

	// Ensure directory exists
	actionDir := filepath.Join(internalGenOutputDir, ".github", "actions", "setup")
	if err := os.MkdirAll(actionDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", actionDir, err)
	}

	outPath := filepath.Join(actionDir, "action.yml")
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

func runInternalGenList(cmd *cobra.Command, args []string) error {
	// List internal templates
	tmplList, err := templates.ListTemplates()
	if err != nil {
		return fmt.Errorf("failed to list internal templates: %w", err)
	}

	fmt.Println("Internal templates (xplat internal gen):")
	for _, t := range tmplList {
		fmt.Printf("  - %s\n", t)
	}

	// List external templates
	extList, err := templates.ListExternalTemplates()
	if err != nil {
		return fmt.Errorf("failed to list external templates: %w", err)
	}

	fmt.Println("\nExternal templates (xplat gen):")
	for _, t := range extList {
		fmt.Printf("  - %s\n", t)
	}
	return nil
}

// === internal:dev - Developer utilities ===

var internalDevCmd = &cobra.Command{
	Use:   "dev",
	Short: "Developer utilities for xplat development",
	Long:  `Commands for developing xplat itself.`,
}

var internalDevBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build xplat and generate all internal files",
	Long: `Build xplat binary and regenerate all self-managed files.

This is the recommended way to build xplat during development.
It ensures all generated files are in sync with config.go.

Steps:
  1. Generate install.sh from template
  2. Generate .github/actions/setup/action.yml from template
  3. Build xplat binary to ~/.local/bin/xplat
  4. Clean stale binaries from non-canonical locations`,
	RunE: runInternalDevBuild,
}

var internalDevInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Build and install xplat from source (quick)",
	Long: `Build xplat from current source and install to ~/.local/bin/xplat.

This is a quick build without regenerating files.
For a full build with regeneration, use 'xplat internal dev build'.

NEVER use 'go install' as it installs to ~/go/bin which causes conflicts.

See: docs/adr/ADR-016-single-install-location.md`,
	RunE: runInternalDevInstall,
}

var internalDevInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show xplat configuration info",
	Long:  `Display configuration values from internal/config/config.go.`,
	RunE:  runInternalDevInfo,
}

func init() {
	internalDevCmd.AddCommand(internalDevBuildCmd)
	internalDevCmd.AddCommand(internalDevInstallCmd)
	internalDevCmd.AddCommand(internalDevInfoCmd)
	InternalCmd.AddCommand(internalDevCmd)
}

func runInternalDevBuild(cmd *cobra.Command, args []string) error {
	fmt.Println("=== xplat internal dev build ===")
	fmt.Println()

	// 1. Generate install.sh
	fmt.Println("1. Generating install.sh...")
	if err := runInternalGenInstall(cmd, args); err != nil {
		return fmt.Errorf("failed to generate install.sh: %w", err)
	}

	// 2. Generate CI action
	fmt.Println("2. Generating .github/actions/setup/action.yml...")
	if err := runInternalGenAction(cmd, args); err != nil {
		return fmt.Errorf("failed to generate action.yml: %w", err)
	}

	// 3. Build and install
	fmt.Println("3. Building and installing xplat...")
	if err := runInternalDevInstall(cmd, args); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("=== Build complete ===")
	return nil
}

func runInternalDevInstall(cmd *cobra.Command, args []string) error {
	installPath := config.XplatCanonicalBin()
	installDir := config.XplatCanonicalDir()

	// Ensure install directory exists
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", installDir, err)
	}

	fmt.Printf("Building xplat...\n")

	// Build from current directory
	buildCmd := exec.Command("go", "build", "-o", installPath, ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Printf("Installed: %s\n", installPath)

	// Clean stale binaries
	cleanStaleBinaries()

	return nil
}

func runInternalDevInfo(cmd *cobra.Command, args []string) error {
	fmt.Println("xplat Configuration Info")
	fmt.Println("========================")
	fmt.Println()
	fmt.Println("Binary Installation:")
	fmt.Printf("  Canonical path:    %s\n", config.XplatCanonicalBin())
	fmt.Printf("  Canonical dir:     %s\n", config.XplatCanonicalDir())
	fmt.Printf("  Stale locations:   %v\n", config.XplatStaleLocations())
	fmt.Println()
	fmt.Println("GitHub:")
	fmt.Printf("  Repository:        %s\n", config.XplatRepo)
	fmt.Printf("  Releases API:      %s\n", config.XplatReleasesAPI)
	fmt.Printf("  Tag prefix:        %s\n", config.XplatTagPrefix)
	fmt.Printf("  Checksum file:     %s\n", config.XplatChecksumFile)
	fmt.Println()
	fmt.Println("Global Directories:")
	fmt.Printf("  XPLAT_HOME:        %s\n", config.XplatHome())
	fmt.Printf("  XPLAT_BIN:         %s\n", config.XplatBin())
	fmt.Printf("  XPLAT_CACHE:       %s\n", config.XplatCache())
	fmt.Println()
	fmt.Println("Internal Templates:")
	tmplList, _ := templates.ListTemplates()
	for _, t := range tmplList {
		fmt.Printf("  - %s\n", t)
	}
	fmt.Println()
	fmt.Println("External Templates:")
	extList, _ := templates.ListExternalTemplates()
	for _, t := range extList {
		fmt.Printf("  - %s\n", t)
	}
	fmt.Println()
	fmt.Println("Runtime:")
	fmt.Printf("  GOOS:              %s\n", runtime.GOOS)
	fmt.Printf("  GOARCH:            %s\n", runtime.GOARCH)
	fmt.Printf("  Is CI:             %v\n", config.IsCI())
	return nil
}

// cleanStaleBinaries removes xplat from non-canonical locations
func cleanStaleBinaries() {
	for _, loc := range config.XplatStaleLocations() {
		if _, err := os.Stat(loc); err == nil {
			if err := os.Remove(loc); err == nil {
				fmt.Printf("Removed stale xplat from %s\n", loc)
			}
		}
	}
}
