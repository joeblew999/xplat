// Package manifest provides types and parsing for xplat.yaml manifests.
package manifest

// Manifest represents an xplat.yaml package manifest.
type Manifest struct {
	APIVersion  string `yaml:"apiVersion"`
	Kind        string `yaml:"kind"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	License     string `yaml:"license"`
	Repo        string `yaml:"repo,omitempty"`     // GitHub repo name (e.g., "plat-rush"), defaults to name
	Language    string `yaml:"language,omitempty"` // Primary language: go, rust, bun (for CI setup)

	Binary       *BinaryConfig            `yaml:"binary,omitempty"`
	Taskfile     *TaskfileConfig          `yaml:"taskfile,omitempty"`
	Processes    map[string]ProcessConfig `yaml:"processes,omitempty"`
	Env          *EnvConfig               `yaml:"env,omitempty"`
	Dependencies *DependenciesConfig      `yaml:"dependencies,omitempty"`
	Gitignore    *GitignoreConfig         `yaml:"gitignore,omitempty"`
	Core         bool                     `yaml:"core,omitempty"` // Core infrastructure package
}

// RepoName returns the GitHub repo name (Repo field or falls back to Name).
func (m *Manifest) RepoName() string {
	if m.Repo != "" {
		return m.Repo
	}
	return m.Name
}

// BinaryConfig defines how to install the package binary.
type BinaryConfig struct {
	Name           string        `yaml:"name"`
	Main           string        `yaml:"main,omitempty"`             // Path to main package (e.g., "./cmd/polyform")
	RunArgs        string        `yaml:"run_args,omitempty"`         // Arguments for user-facing run (e.g., "edit" for polyform)
	ServiceRunArgs string        `yaml:"service_run_args,omitempty"` // Arguments for service/daemon mode (e.g., "edit -launch-browser=false")
	Source         *SourceConfig `yaml:"source"`
}

// SourceConfig defines where to get the binary.
type SourceConfig struct {
	// Go install path (e.g., "github.com/joeblew999/plat-rush")
	Go string `yaml:"go,omitempty"`

	// GitHub release config
	GitHub *GitHubSource `yaml:"github,omitempty"`

	// NPM package name
	NPM string `yaml:"npm,omitempty"`

	// Direct URL (supports {{.OS}} and {{.ARCH}} templates)
	URL string `yaml:"url,omitempty"`

	// Git repository URL for cloning and building from source
	// Use with Version to pin to a specific tag/branch
	Repo string `yaml:"repo,omitempty"`

	// Version/tag to checkout (used with Repo)
	Version string `yaml:"version,omitempty"`
}

// IsExternalRepo returns true if this source uses git clone from external repo.
func (s *SourceConfig) IsExternalRepo() bool {
	return s != nil && s.Repo != ""
}

// GitHubSource defines a GitHub release source.
type GitHubSource struct {
	Repo  string `yaml:"repo"`  // e.g., "joeblew999/plat-rush"
	Asset string `yaml:"asset"` // e.g., "gorush-{{.OS}}-{{.ARCH}}"
}

// TaskfileConfig defines the taskfile for remote include.
type TaskfileConfig struct {
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace,omitempty"`
}

// ProcessConfig defines a process for process-compose.
type ProcessConfig struct {
	Command    string          `yaml:"command"`
	Port       int             `yaml:"port,omitempty"`
	HealthPath string          `yaml:"health_path,omitempty"`
	HTTPS      bool            `yaml:"https,omitempty"`
	Disabled   bool            `yaml:"disabled,omitempty"`
	DependsOn  []string        `yaml:"depends_on,omitempty"`
	Namespace  string          `yaml:"namespace,omitempty"`
	Readiness  *ReadinessProbe `yaml:"readiness,omitempty"`
}

// ReadinessProbe defines health check timing.
type ReadinessProbe struct {
	InitialDelay     int `yaml:"initial_delay,omitempty"`
	Period           int `yaml:"period,omitempty"`
	Timeout          int `yaml:"timeout,omitempty"`
	FailureThreshold int `yaml:"failure_threshold,omitempty"`
}

// EnvConfig defines environment variables.
type EnvConfig struct {
	Required []EnvVar `yaml:"required,omitempty"`
	Optional []EnvVar `yaml:"optional,omitempty"`
}

// EnvVar defines a single environment variable.
type EnvVar struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description,omitempty"`
	Default      string `yaml:"default,omitempty"`
	Instructions string `yaml:"instructions,omitempty"`
}

// DependenciesConfig defines package dependencies.
type DependenciesConfig struct {
	Runtime []string `yaml:"runtime,omitempty"` // Must be running
	Build   []string `yaml:"build,omitempty"`   // Must be installed
}

// GitignoreConfig defines custom gitignore patterns.
type GitignoreConfig struct {
	// Extra patterns to add to .gitignore (in addition to base patterns)
	Patterns []string `yaml:"patterns,omitempty"`
}

// HasBinary returns true if the manifest defines a binary.
func (m *Manifest) HasBinary() bool {
	return m.Binary != nil && m.Binary.Name != ""
}

// HasProcesses returns true if the manifest defines processes.
func (m *Manifest) HasProcesses() bool {
	return len(m.Processes) > 0
}

// HasEnv returns true if the manifest defines environment variables.
func (m *Manifest) HasEnv() bool {
	return m.Env != nil && (len(m.Env.Required) > 0 || len(m.Env.Optional) > 0)
}

// HasGitignore returns true if the manifest defines custom gitignore patterns.
func (m *Manifest) HasGitignore() bool {
	return m.Gitignore != nil && len(m.Gitignore.Patterns) > 0
}

// AllEnvVars returns all environment variables (required + optional).
func (m *Manifest) AllEnvVars() []EnvVar {
	if m.Env == nil {
		return nil
	}
	vars := make([]EnvVar, 0, len(m.Env.Required)+len(m.Env.Optional))
	vars = append(vars, m.Env.Required...)
	vars = append(vars, m.Env.Optional...)
	return vars
}

// TaskfileURL returns the full URL for remote taskfile include.
func (m *Manifest) TaskfileURL(repoURL string) string {
	if m.Taskfile == nil || m.Taskfile.Path == "" {
		return ""
	}
	return repoURL + ".git//" + m.Taskfile.Path + "?ref=" + m.Version
}
