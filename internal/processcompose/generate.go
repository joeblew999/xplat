// Package processcompose provides parsing, validation, and generation for process-compose.yaml files.
package processcompose

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Generator generates and manages process-compose.yaml files.
type Generator struct {
	configPath string
	config     *ProcessCompose
}

// NewGenerator creates a new generator for the given config path.
func NewGenerator(configPath string) *Generator {
	return &Generator{
		configPath: configPath,
	}
}

// NewConfig creates a new empty ProcessCompose config.
func NewConfig() *ProcessCompose {
	return &ProcessCompose{
		Version:   "0.5",
		Processes: make(map[string]*Process),
	}
}

// LoadOrCreate loads the existing config or creates a new one.
func (g *Generator) LoadOrCreate() (*ProcessCompose, error) {
	if _, err := os.Stat(g.configPath); os.IsNotExist(err) {
		g.config = NewConfig()
		return g.config, nil
	}
	return g.Load()
}

// Load reads the existing config file.
func (g *Generator) Load() (*ProcessCompose, error) {
	data, err := os.ReadFile(g.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", g.configPath, err)
	}

	var config ProcessCompose
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", g.configPath, err)
	}

	if config.Processes == nil {
		config.Processes = make(map[string]*Process)
	}

	g.config = &config
	return g.config, nil
}

// Write writes the config to file.
func (g *Generator) Write(config *ProcessCompose) error {
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

// WriteWithHeader writes the config with a header comment.
func (g *Generator) WriteWithHeader(config *ProcessCompose, header string) error {
	dir := filepath.Dir(g.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	output := append([]byte(header), data...)
	if err := os.WriteFile(g.configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", g.configPath, err)
	}

	return nil
}

// ConfigPath returns the config file path.
func (g *Generator) ConfigPath() string {
	return g.configPath
}

// AddProcess adds a process to the config.
func (g *Generator) AddProcess(name string, proc *Process) error {
	config, err := g.LoadOrCreate()
	if err != nil {
		return err
	}

	if _, exists := config.Processes[name]; exists {
		return fmt.Errorf("process %s already exists in %s", name, g.configPath)
	}

	config.Processes[name] = proc
	return g.Write(config)
}

// RemoveProcess removes a process from the config.
func (g *Generator) RemoveProcess(name string) error {
	config, err := g.Load()
	if err != nil {
		return err
	}

	if _, exists := config.Processes[name]; !exists {
		return fmt.Errorf("process %s not found in %s", name, g.configPath)
	}

	delete(config.Processes, name)
	return g.Write(config)
}

// ListProcesses returns the process names in the config.
func (g *Generator) ListProcesses() ([]string, error) {
	config, err := g.Load()
	if err != nil {
		return nil, err
	}

	var names []string
	for name := range config.Processes {
		names = append(names, name)
	}
	return names, nil
}

// ProcessInput is a generic interface for process configuration sources.
// This allows generation from different sources (manifests, registry packages).
type ProcessInput struct {
	Name        string
	Command     string
	Disabled    bool
	Namespace   string
	DependsOn   []string
	Port        int
	HealthPath  string
	HTTPS       bool
	Readiness   *ReadinessConfig
}

// ReadinessConfig holds readiness probe timing configuration.
type ReadinessConfig struct {
	InitialDelay     int
	Period           int
	Timeout          int
	FailureThreshold int
}

// ProcessFromInput creates a Process from a ProcessInput.
func ProcessFromInput(input *ProcessInput) *Process {
	proc := &Process{
		Command:   input.Command,
		Disabled:  input.Disabled,
		Namespace: input.Namespace,
	}

	// Add dependencies
	if len(input.DependsOn) > 0 {
		proc.DependsOn = make(map[string]DepCfg)
		for _, dep := range input.DependsOn {
			proc.DependsOn[dep] = DepCfg{Condition: "process_healthy"}
		}
	}

	// Add readiness probe if port and health path are defined
	if input.Port > 0 && input.HealthPath != "" {
		scheme := "http"
		if input.HTTPS {
			scheme = "https"
		}
		proc.ReadinessProbe = &ReadinessProbe{
			HTTPGet: &HTTPGet{
				Scheme: scheme,
				Host:   "127.0.0.1",
				Port:   input.Port,
				Path:   input.HealthPath,
			},
		}
		if input.Readiness != nil {
			proc.ReadinessProbe.InitialDelaySeconds = input.Readiness.InitialDelay
			proc.ReadinessProbe.PeriodSeconds = input.Readiness.Period
			proc.ReadinessProbe.TimeoutSeconds = input.Readiness.Timeout
			proc.ReadinessProbe.FailureThreshold = input.Readiness.FailureThreshold
		} else {
			// Default readiness timing
			proc.ReadinessProbe.InitialDelaySeconds = 5
			proc.ReadinessProbe.PeriodSeconds = 10
			proc.ReadinessProbe.TimeoutSeconds = 5
			proc.ReadinessProbe.FailureThreshold = 3
		}
	}

	return proc
}

// ProcessFromInputWithAvailability creates a Process with availability config.
func ProcessFromInputWithAvailability(input *ProcessInput, restart string) *Process {
	proc := ProcessFromInput(input)
	proc.Availability = &Availability{
		Restart: restart,
	}
	return proc
}
