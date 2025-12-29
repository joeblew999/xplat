package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/xplat/internal/taskfile"
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
		return taskfile.Generate(taskfilePath, tfOpts)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Taskfile.yml: %v", err))
	}

	// 3. Generate .gitignore
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := generateIfNeeded(gitignorePath, opts, result, func() error {
		return taskfile.GenerateGitignore(gitignorePath, binaryName)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf(".gitignore: %v", err))
	}

	// 4. Generate .github/workflows/ci.yml
	workflowPath := filepath.Join(dir, ".github", "workflows", "ci.yml")
	if err := generateIfNeeded(workflowPath, opts, result, func() error {
		gen := NewGenerator(nil)
		return gen.GenerateWorkflowDir(dir)
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf(".github/workflows/ci.yml: %v", err))
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
