package taskfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/xplat/internal/templates"
)

// GenerateOptions configures Taskfile generation.
type GenerateOptions struct {
	Name           string // Project name (e.g., "plat-caddy")
	Description    string // Project description
	Binary         string // Binary name (e.g., "caddy")
	MainPath       string // Path to main.go (e.g., "./cmd/caddy")
	HasTests       bool   // Include test task
	HasLint        bool   // Include lint task
	Language       string // "go" or "rust" (default: "go")
	RunArgs        string // Arguments for user-facing run (e.g., "edit")
	ServiceRunArgs string // Arguments for service/daemon mode (e.g., "edit -launch-browser=false")

	// External source repo options
	IsExternalRepo bool   // true if binary comes from external git repo
	SourceRepo     string // Git repo URL
	SourceVersion  string // Tag/branch to checkout
}

// DefaultOptions returns sensible defaults for a Go project.
func DefaultOptions(projectDir string) GenerateOptions {
	name := filepath.Base(projectDir)

	// Try to detect binary name from cmd/*/main.go
	binary := ""
	mainPath := ""
	cmdDir := filepath.Join(projectDir, "cmd")
	if entries, err := os.ReadDir(cmdDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				mainFile := filepath.Join(cmdDir, e.Name(), "main.go")
				if _, err := os.Stat(mainFile); err == nil {
					binary = e.Name()
					mainPath = "./cmd/" + e.Name()
					break
				}
			}
		}
	}

	// Check for tests
	hasTests := false
	if matches, _ := filepath.Glob(filepath.Join(projectDir, "**/*_test.go")); len(matches) > 0 {
		hasTests = true
	}
	// Also check root level
	if matches, _ := filepath.Glob(filepath.Join(projectDir, "*_test.go")); len(matches) > 0 {
		hasTests = true
	}

	return GenerateOptions{
		Name:     name,
		Binary:   binary,
		MainPath: mainPath,
		HasTests: hasTests,
		HasLint:  true, // Always include lint
		Language: "go", // Default to Go
	}
}

// Generate creates a Taskfile.yml for a plat-* project.
func Generate(outputPath string, opts GenerateOptions) error {
	// Build the binary name and main path
	binary := opts.Binary
	if binary == "" {
		binary = opts.Name
	}
	mainPath := opts.MainPath
	if mainPath == "" {
		mainPath = "."
	}

	// Default language to "go" if not specified
	language := opts.Language
	if language == "" {
		language = "go"
	}

	content, err := templates.RenderExternal("taskfile.yml.tmpl", templates.TaskfileData{
		Name:           opts.Name,
		Binary:         binary,
		MainPath:       mainPath,
		HasTests:       opts.HasTests,
		Language:       language,
		RunArgs:        opts.RunArgs,
		ServiceRunArgs: opts.ServiceRunArgs,
		IsExternalRepo: opts.IsExternalRepo,
		SourceRepo:     opts.SourceRepo,
		SourceVersion:  opts.SourceVersion,
	})
	if err != nil {
		return fmt.Errorf("failed to render taskfile: %w", err)
	}

	return os.WriteFile(outputPath, content, 0644)
}

// GitignoreOptions configures gitignore generation.
type GitignoreOptions struct {
	BinaryName string   // Name of the binary to ignore at root level
	Patterns   []string // Extra patterns from manifest
}

// GenerateGitignore creates a .gitignore for Go projects.
func GenerateGitignore(outputPath string, binaryName string) error {
	return GenerateGitignoreWithOptions(outputPath, GitignoreOptions{BinaryName: binaryName})
}

// GenerateGitignoreWithOptions creates a .gitignore with custom patterns.
func GenerateGitignoreWithOptions(outputPath string, opts GitignoreOptions) error {
	content, err := templates.RenderExternal("gitignore.tmpl", templates.GitignoreData{
		BinaryName: opts.BinaryName,
		Patterns:   opts.Patterns,
	})
	if err != nil {
		return fmt.Errorf("failed to render gitignore: %w", err)
	}

	return os.WriteFile(outputPath, content, 0644)
}

// GenerateReadme creates a README.md for plat-* projects.
func GenerateReadme(outputPath string, name, description string) error {
	// Default description
	if description == "" {
		description = name + " - a plat-* project"
	}

	content, err := templates.RenderExternal("readme.md.tmpl", templates.ReadmeData{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return fmt.Errorf("failed to render readme: %w", err)
	}

	return os.WriteFile(outputPath, content, 0644)
}

// ServiceTaskfileOptions holds optional process-level config for service taskfile generation.
type ServiceTaskfileOptions struct {
	Port       string
	HealthPath string
}

// GenerateServiceTaskfile creates a taskfiles/Taskfile.service.yml for remote include by consumers.
func GenerateServiceTaskfile(outputPath string, binaryName string, projectName string, opts *ServiceTaskfileOptions) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate binary variable name (uppercase)
	varName := strings.ToUpper(binaryName)

	port := "8080"
	healthPath := ""
	if opts != nil {
		if opts.Port != "" {
			port = opts.Port
		}
		healthPath = opts.HealthPath
	}

	content, err := templates.RenderExternal("service.taskfile.yml.tmpl", templates.ServiceTaskfileData{
		Name:          projectName,
		BinaryName:    binaryName,
		BinaryVarName: varName,
		Port:          port,
		Host:          "0.0.0.0",
		HealthPath:    healthPath,
	})
	if err != nil {
		return fmt.Errorf("failed to render service taskfile: %w", err)
	}

	return os.WriteFile(outputPath, content, 0644)
}
