package manifest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joeblew999/xplat/internal/processcompose"
	"github.com/joeblew999/xplat/internal/syncgh"
	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/joeblew999/xplat/internal/templates"
)

// BootstrapOptions configures the bootstrap process.
type BootstrapOptions struct {
	Force   bool // Overwrite existing files
	DryRun  bool // Just show what would be done
	Verbose bool // Print details
}

// BootstrapResult tracks what was created/updated.
type BootstrapResult struct {
	Created  []string
	Updated  []string
	Skipped  []string
	Errors   []string
	Manifest *Manifest
}

// Bootstrap ensures a directory has all standard plat-* files.
// It creates or updates: xplat.yaml, Taskfile.yml, .gitignore, .github/workflows/ci.yml, README.md
func Bootstrap(dir string, opts BootstrapOptions) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	// Ensure directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory does not exist: %s", dir)
	}

	// 1. Load or create manifest
	manifestPath := filepath.Join(dir, "xplat.yaml")
	var m *Manifest
	loader := NewLoader()

	if _, err := os.Stat(manifestPath); err == nil {
		// Manifest exists, load it
		m, err = loader.LoadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load manifest: %w", err)
		}
		result.Skipped = append(result.Skipped, "xplat.yaml (exists)")
	} else {
		// Create new manifest
		_, err := Init(dir, InitOptions{Force: opts.Force})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("xplat.yaml: %v", err))
		} else {
			// Load the newly created manifest
			m, err = loader.LoadFile(manifestPath)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("xplat.yaml load: %v", err))
			} else {
				result.Created = append(result.Created, "xplat.yaml")
			}
		}
	}

	if m == nil {
		return result, fmt.Errorf("no manifest available")
	}
	result.Manifest = m

	// Get project info for generation
	projectName := m.Name
	if projectName == "" {
		projectName = filepath.Base(dir)
	}
	binaryName := projectName
	if m.Binary != nil && m.Binary.Name != "" {
		binaryName = m.Binary.Name
	}

	// 2. Generate Taskfile.yml
	taskfilePath := filepath.Join(dir, "Taskfile.yml")
	if err := generateIfNeeded(taskfilePath, opts, result, func() error {
		tfOpts := taskfile.DefaultOptions(dir)
		tfOpts.Name = projectName
		if m.Binary != nil && m.Binary.Name != "" {
			tfOpts.Binary = m.Binary.Name
		}
		if m.Binary != nil && m.Binary.Source != nil && m.Binary.Source.Go != "" {
			// Extract main path from go install path
			tfOpts.MainPath = extractMainPath(m.Binary.Source.Go)
		}
		// Pass language from manifest (defaults to "go" in Generate)
		if m.Language != "" {
			tfOpts.Language = m.Language
		}
		// Pass external repo info if present
		if m.Binary != nil && m.Binary.Source != nil && m.Binary.Source.IsExternalRepo() {
			tfOpts.IsExternalRepo = true
			tfOpts.SourceRepo = m.Binary.Source.Repo
			tfOpts.SourceVersion = m.Binary.Source.Version
			// Use Main from binary config if specified
			if m.Binary.Main != "" {
				tfOpts.MainPath = m.Binary.Main
			}
		}
		// Pass run_args if specified
		if m.Binary != nil && m.Binary.RunArgs != "" {
			tfOpts.RunArgs = m.Binary.RunArgs
		}
		// Pass service_run_args if specified
		if m.Binary != nil && m.Binary.ServiceRunArgs != "" {
			tfOpts.ServiceRunArgs = m.Binary.ServiceRunArgs
		}
		return taskfile.Generate(taskfilePath, tfOpts)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Taskfile.yml: %v", err))
	}

	// 3. Generate .gitignore
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := generateIfNeeded(gitignorePath, opts, result, func() error {
		gitOpts := taskfile.GitignoreOptions{BinaryName: binaryName}
		if m.HasGitignore() {
			gitOpts.Patterns = m.Gitignore.Patterns
		}
		return taskfile.GenerateGitignoreWithOptions(gitignorePath, gitOpts)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf(".gitignore: %v", err))
	}

	// 4. Generate .github/workflows/ci.yml
	workflowPath := filepath.Join(dir, ".github", "workflows", "ci.yml")
	if err := generateIfNeeded(workflowPath, opts, result, func() error {
		gen := NewGenerator(nil)
		wfOpts := WorkflowOptions{
			Language: m.Language,
		}
		// Check if this is an external repo project
		if m.Binary != nil && m.Binary.Source != nil && m.Binary.Source.IsExternalRepo() {
			wfOpts.IsExternalRepo = true
		}
		return gen.GenerateWorkflowDirWithOptions(dir, wfOpts)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf(".github/workflows/ci.yml: %v", err))
	}

	// 4b. Generate .github/workflows/pages.yml for GitHub Pages
	pagesPath := filepath.Join(dir, ".github", "workflows", "pages.yml")
	if err := generateIfNeeded(pagesPath, opts, result, func() error {
		content, err := templates.RenderExternal("pages.yml.tmpl", templates.PagesWorkflowData{
			Name: projectName,
		})
		if err != nil {
			return err
		}
		return os.WriteFile(pagesPath, content, 0644)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf(".github/workflows/pages.yml: %v", err))
	}

	// 4c. Generate _config.yml for Jekyll
	jekyllPath := filepath.Join(dir, "_config.yml")
	if err := generateIfNeeded(jekyllPath, opts, result, func() error {
		description := m.Description
		if description == "" {
			description = projectName + " - powered by xplat"
		}
		content, err := templates.RenderExternal("_config.yml.tmpl", templates.JekyllConfigData{
			Name:        projectName,
			Description: description,
		})
		if err != nil {
			return err
		}
		return os.WriteFile(jekyllPath, content, 0644)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("_config.yml: %v", err))
	}

	// 5. Generate README.md (only if missing - don't overwrite custom READMEs)
	readmePath := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		if err := taskfile.GenerateReadme(readmePath, projectName, m.Description); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("README.md: %v", err))
		} else {
			result.Created = append(result.Created, "README.md")
		}
	} else {
		result.Skipped = append(result.Skipped, "README.md (exists)")
	}

	// 6. Generate .env.example if manifest has env vars
	if m.HasEnv() {
		envPath := filepath.Join(dir, ".env.example")
		if err := generateIfNeeded(envPath, opts, result, func() error {
			gen := NewGenerator([]*Manifest{m})
			return gen.GenerateEnvExample(envPath)
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf(".env.example: %v", err))
		}
	}

	// 7. Generate process-compose.yaml if manifest has processes
	if m.HasProcesses() {
		pcPath := filepath.Join(dir, "process-compose.yaml")
		if err := generateIfNeeded(pcPath, opts, result, func() error {
			return generateProcessCompose(pcPath, m)
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("process-compose.yaml: %v", err))
		}
	}

	// 8. Generate taskfiles/Taskfile.service.yml for remote include by consumers
	serviceTaskfilePath := filepath.Join(dir, "taskfiles", "Taskfile.service.yml")
	if err := generateIfNeeded(serviceTaskfilePath, opts, result, func() error {
		// Extract port and health path from the first process with a port
		var svcOpts *taskfile.ServiceTaskfileOptions
		for _, proc := range m.Processes {
			if proc.Port > 0 {
				svcOpts = &taskfile.ServiceTaskfileOptions{
					Port:       fmt.Sprintf("%d", proc.Port),
					HealthPath: strings.TrimPrefix(proc.HealthPath, "/"),
				}
				break
			}
		}
		return taskfile.GenerateServiceTaskfile(serviceTaskfilePath, binaryName, projectName, svcOpts)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("taskfiles/Taskfile.service.yml: %v", err))
	}

	// 9. Create docs/ directory with .gitkeep for GitHub Pages
	docsDir := filepath.Join(dir, "docs")
	docsGitkeep := filepath.Join(docsDir, ".gitkeep")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(docsDir, 0755); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("docs/: %v", err))
		} else {
			// Create .gitkeep to ensure the folder is tracked
			if err := os.WriteFile(docsGitkeep, []byte(""), 0644); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("docs/.gitkeep: %v", err))
			} else {
				result.Created = append(result.Created, "docs/")
			}
		}
	} else {
		result.Skipped = append(result.Skipped, "docs/ (exists)")
	}

	// 10. Enable GitHub Pages via gh CLI (only if in a git repo with remote)
	if err := enableGitHubPages(dir, opts.Verbose); err != nil {
		// Not a fatal error - just note it
		if opts.Verbose {
			result.Errors = append(result.Errors, fmt.Sprintf("GitHub Pages: %v", err))
		}
	} else {
		result.Created = append(result.Created, "GitHub Pages (enabled)")
	}

	return result, nil
}

// generateIfNeeded creates a file if it doesn't exist or if force is set.
func generateIfNeeded(path string, opts BootstrapOptions, result *BootstrapResult, generate func() error) error {
	filename := filepath.Base(path)

	// Check if file exists
	exists := false
	if _, err := os.Stat(path); err == nil {
		exists = true
	}

	if exists && !opts.Force {
		result.Skipped = append(result.Skipped, filename+" (exists)")
		return nil
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	if err := generate(); err != nil {
		return err
	}

	if exists {
		result.Updated = append(result.Updated, filename)
	} else {
		result.Created = append(result.Created, filename)
	}

	return nil
}

// extractMainPath extracts the main package path from a go install path.
// e.g., "github.com/user/repo/cmd/tool@latest" -> "./cmd/tool"
func extractMainPath(goInstall string) string {
	// Remove version suffix
	path := goInstall
	if idx := len(path) - 1; idx > 0 {
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '@' {
				path = path[:i]
				break
			}
		}
	}

	// Find cmd/ or main package
	if idx := findSubstring(path, "/cmd/"); idx >= 0 {
		return "." + path[idx:]
	}

	return "."
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// generateProcessCompose creates a process-compose.yaml from manifest processes.
func generateProcessCompose(path string, m *Manifest) error {
	gen := processcompose.NewGenerator(path)
	config := processcompose.NewConfig()

	// Set env_file to load .env
	config.EnvFile = []string{".env"}

	// Convert manifest processes to process-compose processes
	for name, proc := range m.Processes {
		// Ensure command uses xplat task prefix
		command := proc.Command
		if !strings.HasPrefix(command, "xplat ") {
			// Convert "task foo:run" to "xplat task foo:run"
			if strings.HasPrefix(command, "task ") {
				command = "xplat " + command
			}
		}

		// Dev mode: replace "task run" with "task dev" for hot reload
		if proc.DevMode {
			command = strings.Replace(command, "task run", "task dev", 1)
		}

		pcProc := &processcompose.Process{
			Command:    command,
			WorkingDir: ".",
			Disabled:   proc.Disabled,
			Namespace:  proc.Namespace,
			Shutdown: &processcompose.Shutdown{
				Signal:         15, // SIGTERM
				TimeoutSeconds: 10,
			},
			Availability: &processcompose.Availability{
				Restart:        "on_failure",
				BackoffSeconds: 5,
			},
		}

		// Add dependencies
		if len(proc.DependsOn) > 0 {
			pcProc.DependsOn = make(map[string]processcompose.DepCfg)
			for _, dep := range proc.DependsOn {
				pcProc.DependsOn[dep] = processcompose.DepCfg{Condition: "process_healthy"}
			}
		}

		// Add readiness probe
		if proc.Port > 0 {
			initialDelay := 3
			period := 5
			if proc.Readiness != nil {
				if proc.Readiness.InitialDelay > 0 {
					initialDelay = proc.Readiness.InitialDelay
				}
				if proc.Readiness.Period > 0 {
					period = proc.Readiness.Period
				}
			}
			probe := &processcompose.ReadinessProbe{
				InitialDelaySeconds: initialDelay,
				PeriodSeconds:       period,
			}
			if proc.HealthPath != "" {
				// Use http_get probe when health_path is defined
				scheme := "http"
				if proc.HTTPS {
					scheme = "https"
				}
				probe.HTTPGet = &processcompose.HTTPGet{
					Scheme: scheme,
					Host:   "127.0.0.1",
					Port:   fmt.Sprintf("%d", proc.Port),
					Path:   proc.HealthPath,
				}
			} else {
				// Fall back to exec probe using task health command
				probe.Exec = &processcompose.ExecProbe{
					Command: fmt.Sprintf("xplat task %s:health", name),
				}
			}
			pcProc.ReadinessProbe = probe
		}

		config.Processes[name] = pcProc
	}

	// Write with header comment
	header := fmt.Sprintf(`# ============================================================================
# GENERATED FILE - DO NOT EDIT MANUALLY
# ============================================================================
# Generated by: xplat manifest bootstrap
# Regenerate with: xplat manifest bootstrap --force
# Source: https://github.com/joeblew999/xplat
# ============================================================================
#
# %s Process Compose Configuration
# Run with: xplat process up
# Or use: xplat task up
#
`, m.Name)

	return gen.WriteWithHeader(config, header)
}

// enableGitHubPages attempts to enable GitHub Pages for the repository.
// It tries GITHUB_TOKEN first (no gh CLI needed), then falls back to gh CLI.
func enableGitHubPages(dir string, verbose bool) error {
	// Try to get repo info from git remote
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not a git repo with remote")
	}

	remoteURL := strings.TrimSpace(out.String())
	owner, repo, err := syncgh.GetRepoFromRemote(remoteURL)
	if err != nil {
		return err
	}

	// Try using GITHUB_TOKEN directly (no gh CLI needed)
	if os.Getenv("GITHUB_TOKEN") != "" {
		if err := syncgh.EnablePages(owner, repo); err == nil {
			return nil
		}
		// Fall through to gh CLI
	}

	// Fall back to gh CLI
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("neither GITHUB_TOKEN nor gh CLI available")
	}

	fullRepo := owner + "/" + repo

	// Try to enable GitHub Pages with workflow build type
	// First try POST (create), then PUT (update) if it already exists
	cmd = exec.Command("gh", "api", fmt.Sprintf("repos/%s/pages", fullRepo),
		"--method", "POST", "-f", "build_type=workflow")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		// Try PUT if POST fails (pages might already exist)
		cmd = exec.Command("gh", "api", fmt.Sprintf("repos/%s/pages", fullRepo),
			"--method", "PUT", "-f", "build_type=workflow")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pages already enabled or API error")
		}
	}

	return nil
}

// CheckConformity checks if a directory conforms to plat-* standards.
func CheckConformity(dir string) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	requiredFiles := []string{
		"xplat.yaml",
		"Taskfile.yml",
		".gitignore",
		".github/workflows/ci.yml",
		"README.md",
	}

	for _, file := range requiredFiles {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			result.Errors = append(result.Errors, fmt.Sprintf("missing: %s", file))
		} else {
			result.Skipped = append(result.Skipped, fmt.Sprintf("ok: %s", file))
		}
	}

	// Load manifest to check structure
	manifestPath := filepath.Join(dir, "xplat.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		loader := NewLoader()
		m, err := loader.LoadFile(manifestPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid manifest: %v", err))
		} else {
			result.Manifest = m
		}
	}

	return result, nil
}
