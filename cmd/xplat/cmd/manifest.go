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

	manifestPath := filepath.Join(path, "xplat.yaml")

	// Check if manifest already exists
	if _, err := os.Stat(manifestPath); err == nil && !manifestForce {
		return fmt.Errorf("xplat.yaml already exists (use --force to overwrite)")
	}

	// Detect project info
	name := filepath.Base(path)
	if path == "." {
		if wd, err := os.Getwd(); err == nil {
			name = filepath.Base(wd)
		}
	}

	// Build manifest content
	var content string
	content = "# xplat.yaml - Package manifest for xplat ecosystem\n"
	content += "apiVersion: xplat/v1\n"
	content += "kind: Package\n\n"
	content += fmt.Sprintf("name: %s\n", name)
	content += "version: main\n"
	content += fmt.Sprintf("description: %s package\n", name)
	content += "author: \n"
	content += "license: MIT\n"

	// Detect go.mod for binary config
	goModPath := filepath.Join(path, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		moduleName := detectGoModule(goModPath)
		if moduleName != "" {
			content += "\n# Binary (detected from go.mod)\n"
			content += "binary:\n"
			content += fmt.Sprintf("  name: %s\n", name)
			content += "  source:\n"
			content += fmt.Sprintf("    go: %s\n", moduleName)
			fmt.Printf("  Detected go.mod: %s\n", moduleName)
		}
	}

	// Detect Taskfile.yml
	taskfilePath := filepath.Join(path, "Taskfile.yml")
	if _, err := os.Stat(taskfilePath); err == nil {
		content += "\n# Taskfile (detected)\n"
		content += "taskfile:\n"
		content += "  path: Taskfile.yml\n"
		content += fmt.Sprintf("  namespace: %s\n", name)
		fmt.Printf("  Detected Taskfile.yml\n")
	}

	// Check for taskfiles/ directory
	taskfilesDir := filepath.Join(path, "taskfiles")
	if info, err := os.Stat(taskfilesDir); err == nil && info.IsDir() {
		// Look for Taskfile-*.yml pattern
		files, _ := filepath.Glob(filepath.Join(taskfilesDir, "Taskfile-*.yml"))
		if len(files) > 0 {
			taskfileName := filepath.Base(files[0])
			content += "\n# Taskfile (detected in taskfiles/)\n"
			content += "taskfile:\n"
			content += fmt.Sprintf("  path: taskfiles/%s\n", taskfileName)
			content += fmt.Sprintf("  namespace: %s\n", name)
			fmt.Printf("  Detected taskfiles/%s\n", taskfileName)
		}
	}

	// Add placeholder sections
	content += "\n# Uncomment to define processes for process-compose\n"
	content += "# processes:\n"
	content += "#   server:\n"
	content += "#     command: task run\n"
	content += "#     port: 8080\n"

	content += "\n# Uncomment to define environment variables\n"
	content += "# env:\n"
	content += "#   required:\n"
	content += "#     - name: API_KEY\n"
	content += "#       description: API key for the service\n"

	// Write the manifest
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	fmt.Printf("Created %s\n", manifestPath)
	return nil
}

// detectGoModule reads go.mod and extracts the module name.
func detectGoModule(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	// Simple parsing - look for "module" line
	for _, line := range splitLines(string(data)) {
		if len(line) > 7 && line[:7] == "module " {
			return line[7:]
		}
	}
	return ""
}

// CheckResult holds the result of a manifest check.
type CheckResult struct {
	Name     string
	Path     string
	Errors   []string
	Warnings []string
}

func (r *CheckResult) AddError(msg string) {
	r.Errors = append(r.Errors, msg)
}

func (r *CheckResult) AddWarning(msg string) {
	r.Warnings = append(r.Warnings, msg)
}

func (r *CheckResult) IsValid() bool {
	return len(r.Errors) == 0
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
	var results []CheckResult

	if checkAll {
		// Discover and check all plat-* repos
		manifests, err := loader.DiscoverPlat(path)
		if err != nil {
			return err
		}

		if len(manifests) == 0 {
			fmt.Println("No manifests found in plat-* directories")
			return nil
		}

		// We need to find the actual paths for each manifest
		entries, _ := os.ReadDir(path)
		for _, entry := range entries {
			if !entry.IsDir() || !hasPrefix(entry.Name(), "plat-") {
				continue
			}
			repoPath := filepath.Join(path, entry.Name())
			m, err := loader.LoadDir(repoPath)
			if err != nil {
				continue
			}
			result := checkManifest(m, repoPath)
			results = append(results, result)
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

		result := checkManifest(m, repoPath)
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

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func checkManifest(m *manifest.Manifest, repoPath string) CheckResult {
	result := CheckResult{
		Name: m.Name,
		Path: repoPath,
	}

	// Check taskfile.path exists
	if m.Taskfile != nil && m.Taskfile.Path != "" {
		taskfilePath := filepath.Join(repoPath, m.Taskfile.Path)
		if _, err := os.Stat(taskfilePath); os.IsNotExist(err) {
			result.AddError(fmt.Sprintf("taskfile.path '%s' does not exist", m.Taskfile.Path))
		}
	}

	// Check binary.source.go has go.mod
	if m.Binary != nil && m.Binary.Source != nil && m.Binary.Source.Go != "" {
		goModPath := filepath.Join(repoPath, "go.mod")
		if _, err := os.Stat(goModPath); os.IsNotExist(err) {
			result.AddError("binary.source.go is set but go.mod does not exist")
		}
	}

	// Check processes reference task commands (warning only)
	if len(m.Processes) > 0 {
		// Check if Taskfile exists for task commands
		hasTaskfile := false
		if m.Taskfile != nil && m.Taskfile.Path != "" {
			taskfilePath := filepath.Join(repoPath, m.Taskfile.Path)
			if _, err := os.Stat(taskfilePath); err == nil {
				hasTaskfile = true
			}
		} else {
			// Check for root Taskfile.yml
			if _, err := os.Stat(filepath.Join(repoPath, "Taskfile.yml")); err == nil {
				hasTaskfile = true
			}
		}

		for name, proc := range m.Processes {
			if hasPrefix(proc.Command, "task ") && !hasTaskfile {
				result.AddWarning(fmt.Sprintf("process '%s' uses task command but no Taskfile found", name))
			}
		}
	}

	// Check required env vars have descriptions
	if m.Env != nil {
		for _, v := range m.Env.Required {
			if v.Description == "" {
				result.AddWarning(fmt.Sprintf("required env var '%s' has no description", v.Name))
			}
		}
	}

	// Warn if no description
	if m.Description == "" {
		result.AddWarning("missing description")
	}

	// Warn if no author
	if m.Author == "" {
		result.AddWarning("missing author")
	}

	return result
}
