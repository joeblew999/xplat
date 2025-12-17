// Package process provides process-compose.yaml generation from package registry.
package process

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/xplat/internal/registry"
	"gopkg.in/yaml.v3"
)

// ProcessComposeConfig represents a process-compose.yaml file structure.
type ProcessComposeConfig struct {
	Version   string             `yaml:"version"`
	Processes map[string]Process `yaml:"processes"`
}

// Process represents a single process in process-compose.yaml.
type Process struct {
	Command       string            `yaml:"command"`
	Disabled      bool              `yaml:"disabled,omitempty"`
	DependsOn     map[string]DepCfg `yaml:"depends_on,omitempty"`
	Namespace     string            `yaml:"namespace,omitempty"`
	ReadinessProbe *ReadinessProbe  `yaml:"readiness_probe,omitempty"`
}

// DepCfg represents a dependency configuration.
type DepCfg struct {
	Condition string `yaml:"condition"`
}

// ReadinessProbe represents health check configuration.
type ReadinessProbe struct {
	HTTPGet        *HTTPGet `yaml:"http_get,omitempty"`
	InitialDelay   string   `yaml:"initial_delay_seconds,omitempty"`
	PeriodSeconds  int      `yaml:"period_seconds,omitempty"`
	TimeoutSeconds int      `yaml:"timeout_seconds,omitempty"`
	FailureThresh  int      `yaml:"failure_threshold,omitempty"`
}

// HTTPGet represents HTTP health check configuration.
type HTTPGet struct {
	Scheme string `yaml:"scheme,omitempty"`
	Host   string `yaml:"host,omitempty"`
	Port   int    `yaml:"port"`
	Path   string `yaml:"path"`
}

// Generator generates process-compose.yaml from package registry.
type Generator struct {
	configPath string
	registry   *registry.Client
}

// NewGenerator creates a new process config generator.
func NewGenerator(configPath string) *Generator {
	return &Generator{
		configPath: configPath,
		registry:   registry.NewClient(),
	}
}

// GenerateFromRegistry generates a process-compose.yaml from all packages with process configs.
func (g *Generator) GenerateFromRegistry() (*ProcessComposeConfig, error) {
	packages, err := g.registry.ListPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}

	config := &ProcessComposeConfig{
		Version:   "0.5",
		Processes: make(map[string]Process),
	}

	for _, pkg := range packages {
		if pkg.HasProcess() {
			proc := g.packageToProcess(&pkg)
			config.Processes[pkg.Name] = proc
		}
	}

	return config, nil
}

// AddPackage adds a single package's process to the config file.
func (g *Generator) AddPackage(pkgName string) error {
	pkg, err := g.registry.GetPackage(pkgName)
	if err != nil {
		return fmt.Errorf("failed to get package: %w", err)
	}

	if !pkg.HasProcess() {
		return fmt.Errorf("package %s does not define a process", pkgName)
	}

	config, err := g.loadOrCreate()
	if err != nil {
		return err
	}

	if _, exists := config.Processes[pkgName]; exists {
		return fmt.Errorf("process %s already exists in %s", pkgName, g.configPath)
	}

	config.Processes[pkgName] = g.packageToProcess(pkg)

	return g.save(config)
}

// RemovePackage removes a package's process from the config file.
func (g *Generator) RemovePackage(pkgName string) error {
	config, err := g.load()
	if err != nil {
		return err
	}

	if _, exists := config.Processes[pkgName]; !exists {
		return fmt.Errorf("process %s not found in %s", pkgName, g.configPath)
	}

	delete(config.Processes, pkgName)

	return g.save(config)
}

// ListProcesses returns the processes defined in the config file.
func (g *Generator) ListProcesses() ([]string, error) {
	config, err := g.load()
	if err != nil {
		return nil, err
	}

	var names []string
	for name := range config.Processes {
		names = append(names, name)
	}
	return names, nil
}

// packageToProcess converts a registry package to a process-compose process.
func (g *Generator) packageToProcess(pkg *registry.Package) Process {
	proc := Process{
		Command:   pkg.Process.Command,
		Disabled:  pkg.Process.Disabled,
		Namespace: pkg.Process.Namespace,
	}

	// Add dependencies
	if len(pkg.Process.DependsOn) > 0 {
		proc.DependsOn = make(map[string]DepCfg)
		for _, dep := range pkg.Process.DependsOn {
			proc.DependsOn[dep] = DepCfg{Condition: "process_started"}
		}
	}

	// Add health check if port and path are defined
	if pkg.Process.Port > 0 && pkg.Process.HealthPath != "" {
		proc.ReadinessProbe = &ReadinessProbe{
			HTTPGet: &HTTPGet{
				Scheme: "http",
				Host:   "127.0.0.1",
				Port:   pkg.Process.Port,
				Path:   pkg.Process.HealthPath,
			},
			InitialDelay:   "5s",
			PeriodSeconds:  10,
			TimeoutSeconds: 5,
			FailureThresh:  3,
		}
	}

	return proc
}

// load reads the existing config file.
func (g *Generator) load() (*ProcessComposeConfig, error) {
	data, err := os.ReadFile(g.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", g.configPath, err)
	}

	var config ProcessComposeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", g.configPath, err)
	}

	if config.Processes == nil {
		config.Processes = make(map[string]Process)
	}

	return &config, nil
}

// loadOrCreate loads existing config or creates a new one.
func (g *Generator) loadOrCreate() (*ProcessComposeConfig, error) {
	if _, err := os.Stat(g.configPath); os.IsNotExist(err) {
		return &ProcessComposeConfig{
			Version:   "0.5",
			Processes: make(map[string]Process),
		}, nil
	}
	return g.load()
}

// save writes the config to file.
func (g *Generator) save(config *ProcessComposeConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(g.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(g.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", g.configPath, err)
	}

	return nil
}

// Write writes the config to file (for generate command).
func (g *Generator) Write(config *ProcessComposeConfig) error {
	return g.save(config)
}

// ConfigPath returns the config file path.
func (g *Generator) ConfigPath() string {
	return g.configPath
}
