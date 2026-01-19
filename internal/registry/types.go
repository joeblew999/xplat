// Package registry provides access to the Ubuntu Software package registry.
package registry

// Registry represents the package registry structure from registry.json.
type Registry struct {
	Packages    map[string]Package `json:"packages"`
	GeneratedAt string             `json:"generated_at"`
	RegistryURL string             `json:"registry_url"`
}

// Package represents a package in the registry.
type Package struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description"`
	ImportPath   string         `json:"import_path"`
	RepoURL      string         `json:"repo_url"`
	HasBinary    bool           `json:"has_binary"`
	BinaryName   string         `json:"binary_name"`
	TaskfilePath string         `json:"taskfile_path"`
	License      string         `json:"license"`
	Author       string         `json:"author"`
	Process      *ProcessConfig `json:"process,omitempty"`
}

// ProcessConfig defines how a package runs as a long-running process.
// This enables automatic process-compose.yaml generation from package metadata.
type ProcessConfig struct {
	// Command is the command to run (e.g., "task mailerlite:server")
	Command string `json:"command,omitempty"`

	// Port is the HTTP port the service listens on (e.g., 8086)
	Port int `json:"port,omitempty"`

	// HealthPath is the health check endpoint (e.g., "/health")
	HealthPath string `json:"health_path,omitempty"`

	// Disabled means the process is defined but not started by default
	Disabled bool `json:"disabled,omitempty"`

	// DependsOn lists processes that must start before this one
	DependsOn []string `json:"depends_on,omitempty"`

	// Namespace groups related processes (e.g., "servers", "workers")
	Namespace string `json:"namespace,omitempty"`
}

// HasProcess returns true if the package defines a process configuration.
func (p *Package) HasProcess() bool {
	return p.Process != nil && p.Process.Command != ""
}

// GitHubRepo extracts owner/repo from the repo_url.
// e.g., "https://github.com/joeblew999/ubuntu-website" -> "joeblew999/ubuntu-website"
func (p *Package) GitHubRepo() string {
	// Remove https://github.com/ prefix
	const prefix = "https://github.com/"
	if len(p.RepoURL) > len(prefix) {
		return p.RepoURL[len(prefix):]
	}
	return p.RepoURL
}

// TaskfileURL returns the full URL for the remote taskfile include.
// e.g., "https://github.com/joeblew999/ubuntu-website.git//taskfiles/Taskfile.mailerlite.yml?ref=v0.1.0"
func (p *Package) TaskfileURL() string {
	if p.TaskfilePath == "" {
		return ""
	}
	return "https://github.com/" + p.GitHubRepo() + ".git//" + p.TaskfilePath + "?ref=" + p.Version
}

// Manifest mirrors the xplat.yaml structure for parsing remote manifests.
// This is a subset of manifest.Manifest to avoid import cycles.
type Manifest struct {
	Name        string                    `yaml:"name"`
	Version     string                    `yaml:"version"`
	Description string                    `yaml:"description"`
	Author      string                    `yaml:"author"`
	License     string                    `yaml:"license"`
	Binary      *ManifestBinary           `yaml:"binary,omitempty"`
	Taskfile    *ManifestTF               `yaml:"taskfile,omitempty"`
	Process     *ManifestProc             `yaml:"process,omitempty"`   // Singular (legacy)
	Processes   map[string]*ManifestProc  `yaml:"processes,omitempty"` // Map format (preferred)
}

// GetDefaultProcess returns the "default" process or the first process from the map,
// falling back to the singular Process field for backwards compatibility.
func (m *Manifest) GetDefaultProcess() *ManifestProc {
	// Prefer processes map
	if len(m.Processes) > 0 {
		// Try "default" first
		if proc, ok := m.Processes["default"]; ok {
			return proc
		}
		// Return first process
		for _, proc := range m.Processes {
			return proc
		}
	}
	// Fall back to singular process field
	return m.Process
}

// ManifestBinary is the binary config from xplat.yaml.
type ManifestBinary struct {
	Name   string               `yaml:"name"`
	Source *ManifestBinarySource `yaml:"source,omitempty"`
}

// ManifestBinarySource is the source config for binary installation.
type ManifestBinarySource struct {
	Go     string `yaml:"go,omitempty"`
	GitHub *struct {
		Repo  string `yaml:"repo"`
		Asset string `yaml:"asset,omitempty"`
	} `yaml:"github,omitempty"`
}

// ManifestTF is the taskfile config from xplat.yaml.
type ManifestTF struct {
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace,omitempty"`
}

// ManifestProc is the process config from xplat.yaml.
type ManifestProc struct {
	Command    string   `yaml:"command"`
	Port       int      `yaml:"port,omitempty"`
	HealthPath string   `yaml:"health_path,omitempty"`
	Disabled   bool     `yaml:"disabled,omitempty"`
	DependsOn  []string `yaml:"depends_on,omitempty"`
	Namespace  string   `yaml:"namespace,omitempty"`
}
