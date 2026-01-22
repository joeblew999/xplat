// Package processcompose provides parsing, validation, and generation for process-compose.yaml files.
package processcompose

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// formatPort returns the port as a string, optionally with env var override syntax.
// If envVar is set, returns "${ENVVAR:-default}", otherwise just the port number.
func formatPort(port int, envVar string) string {
	if envVar != "" {
		return fmt.Sprintf("${%s:-%d}", envVar, port)
	}
	return strconv.Itoa(port)
}

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
	Port        int    // Default port value
	PortEnvVar  string // Environment variable name for port override (e.g., "WEB_PORT")
	HealthPath  string
	HTTPS       bool
	Readiness   *ReadinessConfig
	Schedule    *ScheduleConfig // v1.87.0: cron/interval scheduling
}

// ScheduleConfig holds schedule configuration for cron/interval processes.
// Added in process-compose v1.87.0.
type ScheduleConfig struct {
	Cron          string // Cron expression (5 fields: minute hour day month weekday)
	Timezone      string // Timezone for cron (e.g., "UTC", "America/New_York")
	Interval      string // Go duration (e.g., "30s", "5m", "1h")
	RunOnStart    bool   // Run immediately when process-compose starts
	MaxConcurrent int    // Max simultaneous executions (default: 1)
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

		// Format port as string with optional env var override
		portStr := formatPort(input.Port, input.PortEnvVar)

		proc.ReadinessProbe = &ReadinessProbe{
			HTTPGet: &HTTPGet{
				Scheme: scheme,
				Host:   "127.0.0.1",
				Port:   portStr,
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

	// Add schedule if configured (v1.87.0+)
	if input.Schedule != nil {
		proc.Schedule = &Schedule{
			Cron:          input.Schedule.Cron,
			Timezone:      input.Schedule.Timezone,
			Interval:      input.Schedule.Interval,
			RunOnStart:    input.Schedule.RunOnStart,
			MaxConcurrent: input.Schedule.MaxConcurrent,
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
