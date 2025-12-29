package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InitOptions configures manifest initialization.
type InitOptions struct {
	Force   bool   // Overwrite existing manifest
	Name    string // Override detected name
	Author  string // Author name
	License string // License (default: MIT)
}

// InitResult holds the result of manifest initialization.
type InitResult struct {
	Path        string
	Name        string
	DetectedGo  bool
	GoModule    string
	DetectedTask bool
	TaskfilePath string
}

// Init scaffolds a new xplat.yaml manifest for a project.
func Init(path string, opts InitOptions) (*InitResult, error) {
	manifestPath := filepath.Join(path, ManifestFileName)

	// Check if manifest already exists
	if _, err := os.Stat(manifestPath); err == nil && !opts.Force {
		return nil, fmt.Errorf("xplat.yaml already exists (use --force to overwrite)")
	}

	// Detect project info
	name := opts.Name
	if name == "" {
		name = filepath.Base(path)
		if path == "." {
			if wd, err := os.Getwd(); err == nil {
				name = filepath.Base(wd)
			}
		}
	}

	result := &InitResult{
		Path: manifestPath,
		Name: name,
	}

	license := opts.License
	if license == "" {
		license = "MIT"
	}

	// Build manifest content
	var content strings.Builder
	content.WriteString("# xplat.yaml - Package manifest for xplat ecosystem\n")
	content.WriteString("apiVersion: xplat/v1\n")
	content.WriteString("kind: Package\n\n")
	content.WriteString(fmt.Sprintf("name: %s\n", name))
	content.WriteString("version: main\n")
	content.WriteString(fmt.Sprintf("description: %s package\n", name))
	if opts.Author != "" {
		content.WriteString(fmt.Sprintf("author: %s\n", opts.Author))
	} else {
		content.WriteString("author: \n")
	}
	content.WriteString(fmt.Sprintf("license: %s\n", license))

	// Detect go.mod for binary config
	goModPath := filepath.Join(path, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		moduleName := DetectGoModule(goModPath)
		if moduleName != "" {
			result.DetectedGo = true
			result.GoModule = moduleName
			content.WriteString("\n# Binary (detected from go.mod)\n")
			content.WriteString("binary:\n")
			content.WriteString(fmt.Sprintf("  name: %s\n", name))
			content.WriteString("  source:\n")
			content.WriteString(fmt.Sprintf("    go: %s\n", moduleName))
		}
	}

	// Detect Taskfile.yml
	taskfilePath := filepath.Join(path, "Taskfile.yml")
	if _, err := os.Stat(taskfilePath); err == nil {
		result.DetectedTask = true
		result.TaskfilePath = "Taskfile.yml"
		content.WriteString("\n# Taskfile (detected)\n")
		content.WriteString("taskfile:\n")
		content.WriteString("  path: Taskfile.yml\n")
		content.WriteString(fmt.Sprintf("  namespace: %s\n", name))
	}

	// Check for taskfiles/ directory
	taskfilesDir := filepath.Join(path, "taskfiles")
	if info, err := os.Stat(taskfilesDir); err == nil && info.IsDir() {
		// Look for Taskfile-*.yml pattern
		files, _ := filepath.Glob(filepath.Join(taskfilesDir, "Taskfile-*.yml"))
		if len(files) > 0 && !result.DetectedTask {
			taskfileName := filepath.Base(files[0])
			result.DetectedTask = true
			result.TaskfilePath = "taskfiles/" + taskfileName
			content.WriteString("\n# Taskfile (detected in taskfiles/)\n")
			content.WriteString("taskfile:\n")
			content.WriteString(fmt.Sprintf("  path: taskfiles/%s\n", taskfileName))
			content.WriteString(fmt.Sprintf("  namespace: %s\n", name))
		}
	}

	// Add placeholder sections
	content.WriteString("\n# Uncomment to define processes for process-compose\n")
	content.WriteString("# processes:\n")
	content.WriteString("#   server:\n")
	content.WriteString("#     command: task run\n")
	content.WriteString("#     port: 8080\n")

	content.WriteString("\n# Uncomment to define environment variables\n")
	content.WriteString("# env:\n")
	content.WriteString("#   required:\n")
	content.WriteString("#     - name: API_KEY\n")
	content.WriteString("#       description: API key for the service\n")

	// Write the manifest
	if err := os.WriteFile(manifestPath, []byte(content.String()), 0644); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	return result, nil
}

// DetectGoModule reads go.mod and extracts the module name.
func DetectGoModule(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	// Simple parsing - look for "module" line
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(line[7:])
		}
	}
	return ""
}
