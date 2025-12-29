package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckResult holds the result of a manifest validation check.
type CheckResult struct {
	Name     string
	Path     string
	Errors   []string
	Warnings []string
}

// AddError adds an error to the result.
func (r *CheckResult) AddError(msg string) {
	r.Errors = append(r.Errors, msg)
}

// AddWarning adds a warning to the result.
func (r *CheckResult) AddWarning(msg string) {
	r.Warnings = append(r.Warnings, msg)
}

// IsValid returns true if there are no errors.
func (r *CheckResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Check performs deep validation of a manifest against its filesystem.
// repoPath is the root directory where the manifest's referenced files should exist.
func Check(m *Manifest, repoPath string) CheckResult {
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
		hasTaskfile := checkTaskfileExists(m, repoPath)

		for name, proc := range m.Processes {
			if strings.HasPrefix(proc.Command, "task ") && !hasTaskfile {
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

// checkTaskfileExists checks if a Taskfile exists for the manifest.
func checkTaskfileExists(m *Manifest, repoPath string) bool {
	if m.Taskfile != nil && m.Taskfile.Path != "" {
		taskfilePath := filepath.Join(repoPath, m.Taskfile.Path)
		if _, err := os.Stat(taskfilePath); err == nil {
			return true
		}
	}

	// Check for root Taskfile.yml
	if _, err := os.Stat(filepath.Join(repoPath, "Taskfile.yml")); err == nil {
		return true
	}

	return false
}

// CheckAll validates all manifests in plat-* directories.
func CheckAll(loader *Loader, rootPath string) ([]CheckResult, error) {
	var results []CheckResult

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "plat-") {
			continue
		}

		repoPath := filepath.Join(rootPath, entry.Name())
		m, err := loader.LoadDir(repoPath)
		if err != nil {
			continue
		}

		result := Check(m, repoPath)
		results = append(results, result)
	}

	return results, nil
}
