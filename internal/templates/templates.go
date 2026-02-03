// Package templates provides embedded templates for code generation.
//
// Templates are organized into two categories:
//
// 1. xplat/ - Templates for xplat's own files (xplat internal dev build)
//   - install.sh.tmpl - Install script for end users
//   - action.yml.tmpl - GitHub Actions setup action
//   - readme.xplat.md.tmpl - xplat's own README
//   - taskfile.xplat.yml.tmpl - xplat's own Taskfile
//
// 2. project/ - Templates for user projects (xplat gen *)
//   - ci.yml.tmpl - GitHub Actions CI workflow
//   - gitignore.tmpl - .gitignore file
//   - taskfile.yml.tmpl - Taskfile for project tasks
//   - env.example.tmpl - Environment variable example
//   - readme.md.tmpl - Project README
//   - taskfile.generated.yml.tmpl - Generated taskfile with remote includes
//   - process.generated.yml.tmpl - Generated process-compose file
//   - service.taskfile.yml.tmpl - Service taskfile for packages
//
// All templates use values from internal/config/config.go as the source of truth.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed xplat/*.tmpl
var xplatFS embed.FS

//go:embed project/*.tmpl
var projectFS embed.FS

// Common template functions available to all templates.
var commonFuncs = template.FuncMap{
	"splitLines": func(s string) []string {
		return strings.Split(strings.TrimSpace(s), "\n")
	},
}

// render executes a template from the given filesystem.
func render(fs embed.FS, path string, data any) ([]byte, error) {
	content, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", path, err)
	}

	tmpl, err := template.New(path).Funcs(commonFuncs).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", path, err)
	}

	return buf.Bytes(), nil
}

// listTemplates returns template names from a filesystem directory.
func listTemplates(fs embed.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// === Xplat Templates (xplat internal dev build) ===

// RenderXplat renders an xplat template by name.
// These are templates for xplat's own files.
func RenderXplat(name string, data any) ([]byte, error) {
	return render(xplatFS, "xplat/"+name, data)
}

// ListXplatTemplates returns names of all xplat templates.
func ListXplatTemplates() ([]string, error) {
	return listTemplates(xplatFS, "xplat")
}

// RenderInternal is an alias for RenderXplat for backwards compatibility.
func RenderInternal(name string, data any) ([]byte, error) {
	return RenderXplat(name, data)
}

// ListTemplates is an alias for ListXplatTemplates for backwards compatibility.
func ListTemplates() ([]string, error) {
	return ListXplatTemplates()
}

// === Xplat Template Data Types ===

// InstallData holds values for install.sh and action.yml templates.
type InstallData struct {
	UnixInstallDir    string   // ~/.local/bin
	WindowsInstallDir string   // $LOCALAPPDATA/xplat
	BinaryName        string   // xplat
	Repo              string   // joeblew999/xplat
	TagPrefix         string   // xplat-
	ChecksumFile      string   // checksums.txt
	StaleLocations    []string // Locations to clean up
}

// XplatReadmeData holds values for xplat's own README.md template.
type XplatReadmeData struct {
	Categories  []CommandCategory
	AllCommands []CommandInfo
}

// CommandCategory groups commands for README display.
type CommandCategory struct {
	Name        string
	Description string
	Commands    []CommandInfo
}

// CommandInfo holds extracted command metadata.
type CommandInfo struct {
	Name        string
	Short       string
	Long        string
	Use         string
	Subcommands []CommandInfo
}

// XplatTaskfileData holds values for xplat's own Taskfile.yml template.
type XplatTaskfileData struct {
	Commands []CommandInfo
}

// === Project Templates (xplat gen *) ===

// RenderProject renders a project template by name.
// These are templates for user projects that use xplat.
func RenderProject(name string, data any) ([]byte, error) {
	return render(projectFS, "project/"+name, data)
}

// ListProjectTemplates returns names of all project templates.
func ListProjectTemplates() ([]string, error) {
	return listTemplates(projectFS, "project")
}

// RenderExternal is an alias for RenderProject for backwards compatibility.
func RenderExternal(name string, data any) ([]byte, error) {
	return RenderProject(name, data)
}

// ListExternalTemplates is an alias for ListProjectTemplates for backwards compatibility.
func ListExternalTemplates() ([]string, error) {
	return ListProjectTemplates()
}

// === Project Template Data Types ===

// CIWorkflowData holds values for ci.yml template.
type CIWorkflowData struct {
	Language       string // go, rust, bun, or empty
	XplatRepo      string // joeblew999/xplat
	IsExternalRepo bool   // true if binary comes from external git repo (changes CI tasks)

	// xplat-specific options (for xplat's own CI)
	IsXplatSelf   bool   // true if generating CI for xplat itself
	BinaryName    string // binary name for releases (e.g., "xplat")
	TagPrefix     string // tag prefix for releases (e.g., "xplat-" → tags like "xplat-v1.0.0")
	TaskBuild     string // build task name (e.g., "dev:build")
	TaskTest      string // test task name (e.g., "dev:test")
	TaskLint      string // lint task name (e.g., "dev:lint")
	TaskRelease   string // release task name (e.g., "release:build:all")
	SingleOS      bool   // if true, only run on ubuntu-latest (no matrix)
}

// GitignoreData holds values for gitignore template.
type GitignoreData struct {
	BinaryName string
	Patterns   []string
}

// PagesWorkflowData holds values for pages.yml template.
type PagesWorkflowData struct {
	Name string // Project name
}

// JekyllConfigData holds values for _config.yml template.
type JekyllConfigData struct {
	Name        string // Project name
	Description string // Project description
}

// TaskfileData holds values for taskfile.yml template.
type TaskfileData struct {
	Name           string
	Binary         string
	MainPath       string
	HasTests       bool
	Language       string // "go" or "rust"
	RunArgs        string // Arguments for user-facing run (e.g., "edit" for polyform)
	ServiceRunArgs string // Arguments for service/daemon mode (e.g., "edit -launch-browser=false")

	// External source repo fields (for cloning upstream projects)
	IsExternalRepo bool   // true if binary comes from external git repo
	SourceRepo     string // Git repo URL (e.g., "https://github.com/EliCDavis/polyform.git")
	SourceVersion  string // Tag/branch to checkout (e.g., "v0.35.0")
}

// ReadmeData holds values for readme.md template.
type ReadmeData struct {
	Name        string
	Description string
}

// TaskfileGeneratedData holds values for taskfile.generated.yml template.
type TaskfileGeneratedData struct {
	Includes []TaskfileInclude
}

// TaskfileInclude represents a remote taskfile include.
type TaskfileInclude struct {
	Namespace string
	URL       string
}

// ProcessGeneratedData holds values for process.generated.yml template.
type ProcessGeneratedData struct {
	Processes []ProcessDef
}

// ProcessDef represents a process definition.
type ProcessDef struct {
	Name    string
	Command string
}

// EnvExampleData holds values for env.example template.
type EnvExampleData struct {
	Manifests []EnvManifest
}

// EnvManifest represents a manifest for env template rendering.
type EnvManifest struct {
	Name        string
	Description string
	HasEnv      bool
	Env         EnvConfig
}

// EnvConfig holds environment configuration.
type EnvConfig struct {
	Required []EnvVar
	Optional []EnvVar
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name         string
	Description  string
	Instructions string
	Default      string
}

// ServiceTaskfileData holds values for service.taskfile.yml template.
// This generates a reusable taskfile for package developers to expose
// to consumers via remote includes.
type ServiceTaskfileData struct {
	Name          string            // Package name (e.g., "plat-geo")
	BinaryName    string            // Binary name (e.g., "geo")
	BinaryVarName string            // Variable name prefix (e.g., "GEO")
	Port          string            // Default port (e.g., "8086")
	Host          string            // Default host (e.g., "0.0.0.0")
	HealthPath    string            // Health endpoint path without leading slash (e.g., "health")
	ExtraVars     []ServiceExtraVar // Additional variables
}

// ServiceExtraVar represents an extra variable for the service taskfile.
type ServiceExtraVar struct {
	Name    string
	Default string
}
