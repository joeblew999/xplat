// Package projects provides local project registry management.
// This tracks xplat projects registered on the local machine at ~/.xplat/projects.yaml.
package projects

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/xplat/internal/config"
	"gopkg.in/yaml.v3"
)

// Project represents a registered xplat project.
type Project struct {
	Path    string `yaml:"path"`
	Enabled bool   `yaml:"enabled"`
}

// Registry holds the local project registry.
type Registry struct {
	Projects map[string]Project `yaml:"projects"`
}

// Load reads the registry from disk.
// Returns an empty registry if the file doesn't exist.
func Load() (*Registry, error) {
	path := config.XplatProjects()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{
				Projects: make(map[string]Project),
			}, nil
		}
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	if reg.Projects == nil {
		reg.Projects = make(map[string]Project)
	}

	return &reg, nil
}

// Save writes the registry to disk.
func (r *Registry) Save() error {
	path := config.XplatProjects()

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, config.DefaultDirPerms); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	if err := os.WriteFile(path, data, config.DefaultFilePerms); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	return nil
}

// Add adds or updates a project in the registry.
// The project name is derived from the directory name.
// This operation is idempotent.
func (r *Registry) Add(projectPath string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify the path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("project path does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", absPath)
	}

	name := filepath.Base(absPath)
	r.Projects[name] = Project{
		Path:    absPath,
		Enabled: true,
	}

	return name, nil
}

// Remove removes a project from the registry.
func (r *Registry) Remove(name string) error {
	if _, ok := r.Projects[name]; !ok {
		return fmt.Errorf("project %q not found in registry", name)
	}
	delete(r.Projects, name)
	return nil
}

// RemoveByPath removes a project by its path.
func (r *Registry) RemoveByPath(projectPath string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	for name, proj := range r.Projects {
		if proj.Path == absPath {
			delete(r.Projects, name)
			return name, nil
		}
	}

	return "", fmt.Errorf("project at %q not found in registry", absPath)
}

// Enable enables a project in the registry.
func (r *Registry) Enable(name string) error {
	proj, ok := r.Projects[name]
	if !ok {
		return fmt.Errorf("project %q not found in registry", name)
	}
	proj.Enabled = true
	r.Projects[name] = proj
	return nil
}

// Disable disables a project in the registry.
func (r *Registry) Disable(name string) error {
	proj, ok := r.Projects[name]
	if !ok {
		return fmt.Errorf("project %q not found in registry", name)
	}
	proj.Enabled = false
	r.Projects[name] = proj
	return nil
}

// List returns all projects in the registry.
func (r *Registry) List() []Project {
	projects := make([]Project, 0, len(r.Projects))
	for _, proj := range r.Projects {
		projects = append(projects, proj)
	}
	return projects
}

// EnabledProjects returns the paths to all enabled projects.
func (r *Registry) EnabledProjects() []string {
	var paths []string
	for _, proj := range r.Projects {
		if proj.Enabled {
			paths = append(paths, proj.Path)
		}
	}
	return paths
}

// EnabledConfigFiles returns the process-compose config file paths for all enabled projects.
// It searches for config files in priority order defined by config.ProcessComposeSearchOrder().
func (r *Registry) EnabledConfigFiles() []string {
	var configs []string
	searchOrder := config.ProcessComposeSearchOrder()

	for _, proj := range r.Projects {
		if !proj.Enabled {
			continue
		}

		// Find the first existing config file in the project
		for _, name := range searchOrder {
			configPath := filepath.Join(proj.Path, name)
			if _, err := os.Stat(configPath); err == nil {
				configs = append(configs, configPath)
				break
			}
		}
	}

	return configs
}

// Get returns a project by name.
func (r *Registry) Get(name string) (Project, bool) {
	proj, ok := r.Projects[name]
	return proj, ok
}

// GetByPath returns a project by its path.
func (r *Registry) GetByPath(projectPath string) (string, Project, bool) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", Project{}, false
	}

	for name, proj := range r.Projects {
		if proj.Path == absPath {
			return name, proj, true
		}
	}

	return "", Project{}, false
}
