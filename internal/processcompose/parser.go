// Package processcompose provides parsing and validation for process-compose.yaml files.
package processcompose

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProcessCompose represents a parsed process-compose.yaml file.
type ProcessCompose struct {
	Path       string              // File path
	RawContent []byte              // Original file content
	Lines      []string            // Lines for line number lookups
	Version    string              `yaml:"version"`
	EnvFile    []string            `yaml:"env_file"`
	Processes  map[string]*Process `yaml:"processes"`
}

// Process represents a single process definition.
type Process struct {
	Command        string            `yaml:"command"`
	WorkingDir     string            `yaml:"working_dir"`
	Disabled       bool              `yaml:"disabled"`
	Environment    []string          `yaml:"environment"`
	DependsOn      map[string]DepCfg `yaml:"depends_on"`
	Shutdown       *Shutdown         `yaml:"shutdown"`
	Availability   *Availability     `yaml:"availability"`
	ReadinessProbe *ReadinessProbe   `yaml:"readiness_probe"`
	LivenessProbe  *ReadinessProbe   `yaml:"liveness_probe"`
	Namespace      string            `yaml:"namespace"`
}

// DepCfg represents a dependency configuration.
type DepCfg struct {
	Condition string `yaml:"condition"`
}

// Shutdown configuration for graceful shutdown.
type Shutdown struct {
	Signal         int `yaml:"signal"`
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// Availability configuration for restart policies.
type Availability struct {
	Restart        string `yaml:"restart"`
	BackoffSeconds int    `yaml:"backoff_seconds"`
	MaxRestarts    int    `yaml:"max_restarts"`
}

// ReadinessProbe configuration for health checks.
type ReadinessProbe struct {
	Exec                *ExecProbe `yaml:"exec"`
	HTTPGet             *HTTPGet   `yaml:"http_get"`
	InitialDelaySeconds int        `yaml:"initial_delay_seconds"`
	PeriodSeconds       int        `yaml:"period_seconds"`
	TimeoutSeconds      int        `yaml:"timeout_seconds"`
	FailureThreshold    int        `yaml:"failure_threshold"`
}

// ExecProbe for command-based health checks.
type ExecProbe struct {
	Command string `yaml:"command"`
}

// HTTPGet for HTTP-based health checks.
type HTTPGet struct {
	Scheme string `yaml:"scheme"`
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	Path   string `yaml:"path"`
}

// Parse reads and parses a process-compose.yaml file.
func Parse(path string) (*ProcessCompose, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var pc ProcessCompose
	if err := yaml.Unmarshal(content, &pc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	pc.Path = path
	pc.RawContent = content
	pc.Lines = strings.Split(string(content), "\n")

	return &pc, nil
}

// FindLineNumber finds the line number where a pattern first appears.
func (pc *ProcessCompose) FindLineNumber(pattern string) int {
	for i, line := range pc.Lines {
		if strings.Contains(line, pattern) {
			return i + 1 // 1-indexed
		}
	}
	return 0
}

// FindProcessLineNumber finds the line number where a process is defined.
func (pc *ProcessCompose) FindProcessLineNumber(name string) int {
	// Look for "  <name>:" at process indentation level
	pattern := "  " + name + ":"
	for i, line := range pc.Lines {
		if strings.HasPrefix(line, pattern) || line == "    "+name+":" {
			return i + 1
		}
	}
	return pc.FindLineNumber(name + ":")
}

// GetProcess returns a process by name.
func (pc *ProcessCompose) GetProcess(name string) (*Process, bool) {
	p, ok := pc.Processes[name]
	return p, ok
}

// HasEnvFile returns true if env_file is configured.
func (pc *ProcessCompose) HasEnvFile() bool {
	return len(pc.EnvFile) > 0
}

// UsesTaskCommand returns true if the command uses "task <subsystem>:run" pattern.
func (p *Process) UsesTaskCommand() bool {
	return strings.HasPrefix(p.Command, "task ")
}

// UsesTaskHealth returns true if the readiness probe uses "task <subsystem>:health".
func (p *Process) UsesTaskHealth() bool {
	if p.ReadinessProbe == nil || p.ReadinessProbe.Exec == nil {
		return false
	}
	return strings.HasPrefix(p.ReadinessProbe.Exec.Command, "task ") &&
		strings.Contains(p.ReadinessProbe.Exec.Command, ":health")
}

// HasShutdownConfig returns true if shutdown is configured.
func (p *Process) HasShutdownConfig() bool {
	return p.Shutdown != nil && p.Shutdown.Signal > 0
}

// HasRestartPolicy returns true if availability restart is configured.
func (p *Process) HasRestartPolicy() bool {
	return p.Availability != nil && p.Availability.Restart != ""
}

// HasReadinessProbe returns true if any readiness probe is configured.
func (p *Process) HasReadinessProbe() bool {
	if p.ReadinessProbe == nil {
		return false
	}
	return p.ReadinessProbe.Exec != nil || p.ReadinessProbe.HTTPGet != nil
}

// UsesEnvSubstitution returns true if environment vars use ${VAR} syntax.
func (p *Process) UsesEnvSubstitution() bool {
	for _, env := range p.Environment {
		if strings.Contains(env, "${") {
			return true
		}
	}
	return false
}

// GetEnvVars returns a map of environment variable names to their values.
func (p *Process) GetEnvVars() map[string]string {
	result := make(map[string]string)
	for _, env := range p.Environment {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
