package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/xplat/internal/manifest"
	"github.com/spf13/cobra"
)

var (
	manifestDir          string
	manifestOutput       string
	manifestRepoURL      string
	manifestForce        bool
	manifestVerbose      bool
	manifestGitHubOwner  string
	manifestGitHubPrefix string
)

// ManifestCmd is the parent command for manifest operations.
var ManifestCmd = &cobra.Command{
	Use:   "manifest",
	Short: "Work with xplat.yaml manifests",
	Long:  `Load, validate, and generate files from xplat.yaml package manifests.`,
}

var manifestValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate an xplat.yaml manifest",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runManifestValidate,
}

var manifestShowCmd = &cobra.Command{
	Use:   "show [path]",
	Short: "Show manifest details",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runManifestShow,
}

var manifestDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover manifests in plat-* directories",
	RunE:  runManifestDiscover,
}

var manifestDiscoverGitHubCmd = &cobra.Command{
	Use:   "discover-github",
	Short: "Discover manifests from GitHub plat-* repos",
	Long: `Fetch xplat.yaml manifests from GitHub repositories.

Scans all repos matching the pattern (default: plat-*) and fetches
their xplat.yaml manifests.

Examples:
  xplat manifest discover-github                         # From joeblew999/plat-*
  xplat manifest discover-github --owner=myorg           # From myorg/plat-*
  xplat manifest discover-github --owner=myorg --prefix=my-  # From myorg/my-*`,
	RunE: runManifestDiscoverGitHub,
}

var manifestGenEnvCmd = &cobra.Command{
	Use:   "gen-env",
	Short: "Generate .env.example from manifests",
	RunE:  runManifestGenEnv,
}

var manifestGenProcessCmd = &cobra.Command{
	Use:   "gen-process",
	Short: "Generate process-compose.yaml from manifests",
	RunE:  runManifestGenProcess,
}

var manifestGenTaskfileCmd = &cobra.Command{
	Use:   "gen-taskfile",
	Short: "Generate Taskfile.yml with remote includes",
	RunE:  runManifestGenTaskfile,
}

var manifestGenAllCmd = &cobra.Command{
	Use:   "gen-all",
	Short: "Generate all files from manifests",
	RunE:  runManifestGenAll,
}

var manifestGenWorkflowCmd = &cobra.Command{
	Use:   "gen-workflow",
	Short: "Generate unified GitHub Actions CI workflow",
	Long: `Generate a minimal CI workflow that delegates to Taskfile.

This creates .github/workflows/ci.yml with:
- Go setup
- Task installation
- Calls to: task build, task test, task lint

The same commands work locally and in CI.`,
	RunE: runManifestGenWorkflow,
}

var manifestInstallCmd = &cobra.Command{
	Use:   "install [path]",
	Short: "Install binary from manifest",
	Long: `Install the binary defined in an xplat.yaml manifest.

Supports multiple installation sources:
- go: Uses 'go install' (e.g., go: github.com/user/repo/cmd/tool)
- github: Downloads from GitHub releases
- npm: Uses npm/bun global install
- url: Direct download from URL

Examples:
  xplat manifest install                    # Install from ./xplat.yaml
  xplat manifest install /path/to/project   # Install from specific path
  xplat manifest install --force            # Force reinstall`,
	Args: cobra.MaximumNArgs(1),
	RunE: runManifestInstall,
}

var manifestInstallAllCmd = &cobra.Command{
	Use:   "install-all",
	Short: "Install binaries from all discovered manifests",
	RunE:  runManifestInstallAll,
}

var manifestCheckCmd = &cobra.Command{
	Use:   "check [path]",
	Short: "Deep validation of manifest against filesystem",
	Long: `Validate that manifest references match actual files.

Checks:
- taskfile.path exists in the repo
- binary.source.go has a go.mod
- processes reference valid task commands

Examples:
  xplat manifest check                    # Check current directory
  xplat manifest check /path/to/project   # Check specific path
  xplat manifest check -d /workspace      # Check all plat-* repos`,
	Args: cobra.MaximumNArgs(1),
	RunE: runManifestCheck,
}

var manifestInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a new xplat.yaml manifest",
	Long: `Scaffold a new xplat.yaml manifest for a project.

Detects existing files and suggests configuration:
- go.mod → binary config with go install source
- Taskfile.yml → taskfile config for remote includes

Examples:
  xplat manifest init                    # Initialize in current directory
  xplat manifest init /path/to/project   # Initialize in specific path
  xplat manifest init --force            # Overwrite existing manifest`,
	Args: cobra.MaximumNArgs(1),
	RunE: runManifestInit,
}

var manifestBootstrapCmd = &cobra.Command{
	Use:   "bootstrap [path]",
	Short: "Bootstrap a plat-* repository with standard files",
	Long: `Ensure a directory has all standard plat-* files.

Creates or updates:
- xplat.yaml      - Package manifest
- Taskfile.yml    - Standard build/test/lint tasks
- .gitignore      - Go project ignores
- .github/workflows/ci.yml - Unified CI workflow
- README.md       - Basic documentation
- .env.example    - Environment template (if env vars defined)

Examples:
  xplat manifest bootstrap                  # Bootstrap current directory
  xplat manifest bootstrap /path/to/repo    # Bootstrap specific path
  xplat manifest bootstrap --force          # Overwrite existing files
  xplat manifest bootstrap --check          # Just check conformity`,
	Args: cobra.MaximumNArgs(1),
	RunE: runManifestBootstrap,
}

var manifestBootstrapCheck bool

func init() {
	// Flags for discover/gen commands
	ManifestCmd.PersistentFlags().StringVarP(&manifestDir, "dir", "d", ".", "Directory to search for manifests")
	ManifestCmd.PersistentFlags().StringVarP(&manifestOutput, "output", "o", ".", "Output directory for generated files")
	ManifestCmd.PersistentFlags().StringVar(&manifestRepoURL, "repo-url", "https://github.com/joeblew999", "Base URL for GitHub repos")

	// GitHub discovery flags
	manifestDiscoverGitHubCmd.Flags().StringVar(&manifestGitHubOwner, "owner", "joeblew999", "GitHub owner/org")
	manifestDiscoverGitHubCmd.Flags().StringVar(&manifestGitHubPrefix, "prefix", "plat-", "Repo name prefix to match")

	ManifestCmd.AddCommand(manifestValidateCmd)
	ManifestCmd.AddCommand(manifestShowCmd)
	ManifestCmd.AddCommand(manifestDiscoverCmd)
	ManifestCmd.AddCommand(manifestDiscoverGitHubCmd)
	ManifestCmd.AddCommand(manifestGenEnvCmd)
	ManifestCmd.AddCommand(manifestGenProcessCmd)
	ManifestCmd.AddCommand(manifestGenTaskfileCmd)
	ManifestCmd.AddCommand(manifestGenAllCmd)
	ManifestCmd.AddCommand(manifestGenWorkflowCmd)

	// Install commands
	manifestInstallCmd.Flags().BoolVarP(&manifestForce, "force", "f", false, "Force reinstall")
	manifestInstallCmd.Flags().BoolVarP(&manifestVerbose, "verbose", "v", false, "Verbose output")
	manifestInstallAllCmd.Flags().BoolVarP(&manifestForce, "force", "f", false, "Force reinstall")
	manifestInstallAllCmd.Flags().BoolVarP(&manifestVerbose, "verbose", "v", false, "Verbose output")

	ManifestCmd.AddCommand(manifestInstallCmd)
	ManifestCmd.AddCommand(manifestInstallAllCmd)

	// Init command
	manifestInitCmd.Flags().BoolVarP(&manifestForce, "force", "f", false, "Overwrite existing manifest")
	ManifestCmd.AddCommand(manifestInitCmd)

	// Check command
	ManifestCmd.AddCommand(manifestCheckCmd)

	// Bootstrap command
	manifestBootstrapCmd.Flags().BoolVarP(&manifestForce, "force", "f", false, "Overwrite existing files")
	manifestBootstrapCmd.Flags().BoolVar(&manifestBootstrapCheck, "check", false, "Just check conformity, don't create files")
	ManifestCmd.AddCommand(manifestBootstrapCmd)
}

func runManifestValidate(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	loader := manifest.NewLoader()

	// Check if path is a file or directory
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	var m *manifest.Manifest
	if info.IsDir() {
		m, err = loader.LoadDir(path)
	} else {
		m, err = loader.LoadFile(path)
	}

	if err != nil {
		return err
	}

	fmt.Printf("✓ Valid manifest: %s v%s\n", m.Name, m.Version)
	return nil
}

func runManifestShow(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	loader := manifest.NewLoader()

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	var m *manifest.Manifest
	if info.IsDir() {
		m, err = loader.LoadDir(path)
	} else {
		m, err = loader.LoadFile(path)
	}

	if err != nil {
		return err
	}

	fmt.Printf("Name:        %s\n", m.Name)
	fmt.Printf("Version:     %s\n", m.Version)
	fmt.Printf("Description: %s\n", m.Description)
	fmt.Printf("Author:      %s\n", m.Author)
	fmt.Printf("License:     %s\n", m.License)

	if m.HasBinary() {
		fmt.Printf("\nBinary:\n")
		fmt.Printf("  Name: %s\n", m.Binary.Name)
		if m.Binary.Source != nil {
			if m.Binary.Source.Go != "" {
				fmt.Printf("  Source: go install %s\n", m.Binary.Source.Go)
			}
		}
	}

	if m.Taskfile != nil {
		fmt.Printf("\nTaskfile:\n")
		fmt.Printf("  Path: %s\n", m.Taskfile.Path)
		fmt.Printf("  Namespace: %s\n", m.Taskfile.Namespace)
	}

	if m.HasProcesses() {
		fmt.Printf("\nProcesses:\n")
		for name, p := range m.Processes {
			fmt.Printf("  %s:\n", name)
			fmt.Printf("    Command: %s\n", p.Command)
			if p.Port > 0 {
				fmt.Printf("    Port: %d\n", p.Port)
			}
			if len(p.DependsOn) > 0 {
				fmt.Printf("    Depends: %v\n", p.DependsOn)
			}
		}
	}

	if m.HasEnv() {
		fmt.Printf("\nEnvironment Variables:\n")
		fmt.Printf("  Required: %d\n", len(m.Env.Required))
		for _, v := range m.Env.Required {
			fmt.Printf("    - %s\n", v.Name)
		}
		fmt.Printf("  Optional: %d\n", len(m.Env.Optional))
		for _, v := range m.Env.Optional {
			fmt.Printf("    - %s\n", v.Name)
		}
	}

	if m.Dependencies != nil {
		if len(m.Dependencies.Build) > 0 {
			fmt.Printf("\nBuild Dependencies: %v\n", m.Dependencies.Build)
		}
		if len(m.Dependencies.Runtime) > 0 {
			fmt.Printf("Runtime Dependencies: %v\n", m.Dependencies.Runtime)
		}
	}

	return nil
}

func runManifestDiscover(cmd *cobra.Command, args []string) error {
	loader := manifest.NewLoader()

	manifests, err := loader.DiscoverPlat(manifestDir)
	if err != nil {
		return err
	}

	if len(manifests) == 0 {
		fmt.Println("No manifests found in plat-* directories")
		return nil
	}

	fmt.Printf("Found %d manifests:\n\n", len(manifests))
	for _, m := range manifests {
		fmt.Printf("  %s v%s\n", m.Name, m.Version)
		if m.Description != "" {
			fmt.Printf("    %s\n", m.Description)
		}
	}

	return nil
}

func runManifestDiscoverGitHub(cmd *cobra.Command, args []string) error {
	loader := manifest.NewLoader()

	fmt.Printf("Discovering manifests from github.com/%s/%s*...\n\n", manifestGitHubOwner, manifestGitHubPrefix)

	manifests, err := loader.DiscoverGitHub(manifestGitHubOwner, manifestGitHubPrefix)
	if err != nil {
		return err
	}

	if len(manifests) == 0 {
		fmt.Printf("No manifests found in %s/%s* repos\n", manifestGitHubOwner, manifestGitHubPrefix)
		return nil
	}

	fmt.Printf("Found %d manifests:\n\n", len(manifests))
	for _, m := range manifests {
		fmt.Printf("  %s v%s\n", m.Name, m.Version)
		if m.Description != "" {
			fmt.Printf("    %s\n", m.Description)
		}
		if m.HasBinary() {
			fmt.Printf("    Binary: %s\n", m.Binary.Name)
		}
		if m.HasProcesses() {
			fmt.Printf("    Processes: %d\n", len(m.Processes))
		}
	}

	return nil
}

func loadManifestsForGen() ([]*manifest.Manifest, error) {
	loader := manifest.NewLoader()

	// First try to load from current directory
	if m, err := loader.LoadDir(manifestDir); err == nil {
		return []*manifest.Manifest{m}, nil
	}

	// Otherwise discover from plat-* directories
	manifests, err := loader.DiscoverPlat(manifestDir)
	if err != nil {
		return nil, err
	}

	if len(manifests) == 0 {
		return nil, fmt.Errorf("no manifests found")
	}

	return manifests, nil
}

func runManifestGenEnv(cmd *cobra.Command, args []string) error {
	manifests, err := loadManifestsForGen()
	if err != nil {
		return err
	}

	gen := manifest.NewGenerator(manifests)
	outputPath := filepath.Join(manifestOutput, ".env.example")

	if err := gen.GenerateEnvExample(outputPath); err != nil {
		return err
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func runManifestGenProcess(cmd *cobra.Command, args []string) error {
	manifests, err := loadManifestsForGen()
	if err != nil {
		return err
	}

	gen := manifest.NewGenerator(manifests)
	outputPath := filepath.Join(manifestOutput, "process-compose.generated.yaml")

	if err := gen.GenerateProcessCompose(outputPath); err != nil {
		return err
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func runManifestGenTaskfile(cmd *cobra.Command, args []string) error {
	manifests, err := loadManifestsForGen()
	if err != nil {
		return err
	}

	gen := manifest.NewGenerator(manifests)
	outputPath := filepath.Join(manifestOutput, "Taskfile.generated.yml")

	if err := gen.GenerateTaskfile(outputPath, manifestRepoURL); err != nil {
		return err
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func runManifestGenAll(cmd *cobra.Command, args []string) error {
	manifests, err := loadManifestsForGen()
	if err != nil {
		return err
	}

	gen := manifest.NewGenerator(manifests)

	// Generate .env.example
	envPath := filepath.Join(manifestOutput, ".env.example")
	if err := gen.GenerateEnvExample(envPath); err != nil {
		return fmt.Errorf("failed to generate .env.example: %w", err)
	}
	fmt.Printf("Generated %s\n", envPath)

	// Generate process-compose.yaml
	processPath := filepath.Join(manifestOutput, "process-compose.generated.yaml")
	if err := gen.GenerateProcessCompose(processPath); err != nil {
		return fmt.Errorf("failed to generate process-compose: %w", err)
	}
	fmt.Printf("Generated %s\n", processPath)

	// Generate Taskfile
	taskfilePath := filepath.Join(manifestOutput, "Taskfile.generated.yml")
	if err := gen.GenerateTaskfile(taskfilePath, manifestRepoURL); err != nil {
		return fmt.Errorf("failed to generate Taskfile: %w", err)
	}
	fmt.Printf("Generated %s\n", taskfilePath)

	return nil
}

func runManifestGenWorkflow(cmd *cobra.Command, args []string) error {
	// Use current directory or specified output
	baseDir := manifestOutput
	if baseDir == "" {
		baseDir = "."
	}

	gen := manifest.NewGenerator(nil) // No manifests needed for workflow generation
	if err := gen.GenerateWorkflowDir(baseDir); err != nil {
		return fmt.Errorf("failed to generate workflow: %w", err)
	}

	fmt.Printf("Generated %s/.github/workflows/ci.yml\n", baseDir)
	return nil
}

func runManifestInstall(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	loader := manifest.NewLoader()

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	var m *manifest.Manifest
	if info.IsDir() {
		m, err = loader.LoadDir(path)
	} else {
		m, err = loader.LoadFile(path)
	}

	if err != nil {
		return err
	}

	if !m.HasBinary() {
		return fmt.Errorf("manifest %s has no binary defined", m.Name)
	}

	installer := manifest.NewInstaller().
		WithForce(manifestForce).
		WithVerbose(manifestVerbose)

	return installer.Install(m)
}

func runManifestInstallAll(cmd *cobra.Command, args []string) error {
	loader := manifest.NewLoader()

	manifests, err := loader.DiscoverPlat(manifestDir)
	if err != nil {
		return err
	}

	if len(manifests) == 0 {
		fmt.Println("No manifests found in plat-* directories")
		return nil
	}

	installer := manifest.NewInstaller().
		WithForce(manifestForce).
		WithVerbose(manifestVerbose)

	var installed, skipped, failed int

	for _, m := range manifests {
		if !m.HasBinary() {
			skipped++
			continue
		}

		fmt.Printf("Installing %s...\n", m.Name)
		if err := installer.Install(m); err != nil {
			fmt.Printf("  Failed: %v\n", err)
			failed++
		} else {
			installed++
		}
	}

	fmt.Printf("\nSummary: %d installed, %d skipped (no binary), %d failed\n",
		installed, skipped, failed)

	return nil
}

func runManifestInit(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	opts := manifest.InitOptions{
		Force: manifestForce,
	}

	result, err := manifest.Init(path, opts)
	if err != nil {
		return err
	}

	// Print what was detected
	if result.DetectedGo {
		fmt.Printf("  Detected go.mod: %s\n", result.GoModule)
	}
	if result.DetectedTask {
		fmt.Printf("  Detected %s\n", result.TaskfilePath)
	}

	fmt.Printf("Created %s\n", result.Path)
	return nil
}

func runManifestCheck(cmd *cobra.Command, args []string) error {
	// Check if we should check all plat-* repos or a single path
	checkAll := false
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// If -d flag is set and path is ".", check all plat-* repos
	if manifestDir != "." && path == "." {
		checkAll = true
		path = manifestDir
	}

	loader := manifest.NewLoader()
	var results []manifest.CheckResult

	if checkAll {
		// Use internal CheckAll
		var err error
		results, err = manifest.CheckAll(loader, path)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No manifests found in plat-* directories")
			return nil
		}
	} else {
		// Check single path
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("failed to stat path: %w", err)
		}

		var m *manifest.Manifest
		var repoPath string
		if info.IsDir() {
			m, err = loader.LoadDir(path)
			repoPath = path
		} else {
			m, err = loader.LoadFile(path)
			repoPath = filepath.Dir(path)
		}

		if err != nil {
			return err
		}

		result := manifest.Check(m, repoPath)
		results = append(results, result)
	}

	// Print results
	var hasErrors bool
	for _, r := range results {
		if r.IsValid() && len(r.Warnings) == 0 {
			fmt.Printf("✓ %s (%s)\n", r.Name, r.Path)
		} else {
			if !r.IsValid() {
				hasErrors = true
				fmt.Printf("✗ %s (%s)\n", r.Name, r.Path)
				for _, e := range r.Errors {
					fmt.Printf("  ERROR: %s\n", e)
				}
			} else {
				fmt.Printf("⚠ %s (%s)\n", r.Name, r.Path)
			}
			for _, w := range r.Warnings {
				fmt.Printf("  WARN: %s\n", w)
			}
		}
	}

	if hasErrors {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func runManifestBootstrap(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Check-only mode
	if manifestBootstrapCheck {
		result, err := manifest.CheckConformity(path)
		if err != nil {
			return err
		}

		fmt.Println("=== Conformity Check ===")
		for _, msg := range result.Skipped {
			fmt.Printf("  %s\n", msg)
		}
		for _, msg := range result.Errors {
			fmt.Printf("  %s\n", msg)
		}

		if len(result.Errors) > 0 {
			fmt.Printf("\nRun 'xplat manifest bootstrap' to fix missing files.\n")
			return fmt.Errorf("%d issues found", len(result.Errors))
		}

		fmt.Println("\nAll standard files present.")
		return nil
	}

	// Bootstrap mode
	opts := manifest.BootstrapOptions{
		Force:   manifestForce,
		Verbose: manifestVerbose,
	}

	result, err := manifest.Bootstrap(path, opts)
	if err != nil {
		return err
	}

	// Print summary
	if len(result.Created) > 0 {
		fmt.Println("Created:")
		for _, f := range result.Created {
			fmt.Printf("  + %s\n", f)
		}
	}

	if len(result.Updated) > 0 {
		fmt.Println("Updated:")
		for _, f := range result.Updated {
			fmt.Printf("  ~ %s\n", f)
		}
	}

	if len(result.Skipped) > 0 && manifestVerbose {
		fmt.Println("Skipped:")
		for _, f := range result.Skipped {
			fmt.Printf("  - %s\n", f)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  ! %s\n", e)
		}
		return fmt.Errorf("%d errors during bootstrap", len(result.Errors))
	}

	fmt.Printf("\nBootstrap complete for %s\n", path)
	return nil
}
