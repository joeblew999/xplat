package processcompose

import (
	"fmt"
	"regexp"
	"strings"
)

// Severity indicates how serious a violation is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warn"
)

// Violation represents a rule violation found in a process-compose file.
type Violation struct {
	File     string   // File path
	Line     int      // Line number (1-indexed, 0 if unknown)
	Column   int      // Column number (1-indexed, 0 if unknown)
	Rule     string   // Rule identifier
	Message  string   // Human-readable message
	Severity Severity // error or warn
	Fixable  bool     // Can be auto-fixed by fmt
}

// String formats the violation for display.
func (v Violation) String() string {
	loc := v.File
	if v.Line > 0 {
		loc = fmt.Sprintf("%s:%d", v.File, v.Line)
		if v.Column > 0 {
			loc = fmt.Sprintf("%s:%d:%d", v.File, v.Line, v.Column)
		}
	}
	return fmt.Sprintf("%s: %s: %s (%s)", loc, v.Severity, v.Message, v.Rule)
}

// Rule defines a validation rule for process-compose files.
type Rule interface {
	Name() string
	Description() string
	Check(pc *ProcessCompose) []Violation
}

// FmtRule defines an auto-fixable formatting rule.
type FmtRule interface {
	Rule
	Fix(pc *ProcessCompose) ([]byte, error) // Returns modified content
}

// ===== Lint Rules =====
// These rules enforce xplat conventions for process-compose files.

// TaskCommandRule checks that commands use "task <subsystem>:run" pattern.
// This ensures Process Compose only orchestrates - all implementation lives in Taskfiles.
type TaskCommandRule struct{}

func (r TaskCommandRule) Name() string { return "task-command" }
func (r TaskCommandRule) Description() string {
	return "Commands should use 'task <subsystem>:run' pattern"
}

func (r TaskCommandRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	for name, proc := range pc.Processes {
		if proc.Disabled {
			continue // Skip disabled processes
		}

		if !proc.UsesTaskCommand() {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("process '%s' should use 'task %s:run' instead of '%s'", name, name, proc.Command),
				Severity: SeverityWarning,
			})
		} else {
			// Check if it uses :run suffix (not :dev:run or other)
			cmd := proc.Command
			if !strings.Contains(cmd, ":run") {
				violations = append(violations, Violation{
					File:     pc.Path,
					Line:     pc.FindProcessLineNumber(name),
					Rule:     r.Name(),
					Message:  fmt.Sprintf("process '%s' command should include ':run' suffix", name),
					Severity: SeverityWarning,
				})
			}
		}
	}

	return violations
}

// TaskHealthRule checks that readiness probes use "task <subsystem>:health" pattern.
type TaskHealthRule struct{}

func (r TaskHealthRule) Name() string { return "task-health" }
func (r TaskHealthRule) Description() string {
	return "Readiness probes should use 'task <subsystem>:health' pattern"
}

func (r TaskHealthRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	for name, proc := range pc.Processes {
		if proc.Disabled {
			continue
		}

		// Only check if readiness probe exists with exec
		if proc.ReadinessProbe != nil && proc.ReadinessProbe.Exec != nil {
			if !proc.UsesTaskHealth() {
				violations = append(violations, Violation{
					File:     pc.Path,
					Line:     pc.FindProcessLineNumber(name),
					Rule:     r.Name(),
					Message:  fmt.Sprintf("process '%s' readiness probe should use 'task %s:health'", name, name),
					Severity: SeverityWarning,
				})
			}
		}
	}

	return violations
}

// ShutdownConfigRule checks that processes have shutdown configuration.
type ShutdownConfigRule struct{}

func (r ShutdownConfigRule) Name() string { return "shutdown-config" }
func (r ShutdownConfigRule) Description() string {
	return "Processes should have shutdown signal and timeout configuration"
}

func (r ShutdownConfigRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	for name, proc := range pc.Processes {
		if proc.Disabled {
			continue
		}

		if !proc.HasShutdownConfig() {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("process '%s' should have shutdown: with signal and timeout_seconds", name),
				Severity: SeverityWarning,
			})
		}
	}

	return violations
}

// RestartPolicyRule checks that processes have restart policy.
type RestartPolicyRule struct{}

func (r RestartPolicyRule) Name() string { return "restart-policy" }
func (r RestartPolicyRule) Description() string {
	return "Processes should have availability restart policy"
}

func (r RestartPolicyRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	for name, proc := range pc.Processes {
		if proc.Disabled {
			continue
		}

		if !proc.HasRestartPolicy() {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("process '%s' should have availability: restart policy", name),
				Severity: SeverityWarning,
			})
		}
	}

	return violations
}

// EnvSubstitutionRule checks that environment variables use ${VAR} substitution.
type EnvSubstitutionRule struct{}

func (r EnvSubstitutionRule) Name() string { return "env-substitution" }
func (r EnvSubstitutionRule) Description() string {
	return "Environment variables with secrets should use ${VAR} substitution from .env"
}

func (r EnvSubstitutionRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	// Patterns that suggest secrets or configurable values
	secretPatterns := []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "API_", "AUTH_", "ACCOUNT_ID"}

	for name, proc := range pc.Processes {
		if proc.Disabled {
			continue
		}

		for _, env := range proc.Environment {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}
			varName := parts[0]
			varValue := parts[1]

			// Check if this looks like a secret but isn't using substitution
			for _, pattern := range secretPatterns {
				if strings.Contains(varName, pattern) {
					if !strings.Contains(varValue, "${") {
						violations = append(violations, Violation{
							File:     pc.Path,
							Line:     pc.FindProcessLineNumber(name),
							Rule:     r.Name(),
							Message:  fmt.Sprintf("process '%s': %s should use ${%s} substitution from .env", name, varName, varName),
							Severity: SeverityWarning,
						})
					}
					break
				}
			}
		}
	}

	return violations
}

// EnvFileRule checks that env_file is configured for projects with secrets.
type EnvFileRule struct{}

func (r EnvFileRule) Name() string { return "env-file" }
func (r EnvFileRule) Description() string {
	return "Process-compose should load env_file for variable substitution"
}

func (r EnvFileRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	// Check if any process uses ${VAR} substitution
	usesSubstitution := false
	for _, proc := range pc.Processes {
		if proc.UsesEnvSubstitution() {
			usesSubstitution = true
			break
		}
	}

	// If substitution is used but env_file is not configured
	if usesSubstitution && !pc.HasEnvFile() {
		violations = append(violations, Violation{
			File:     pc.Path,
			Line:     1,
			Rule:     r.Name(),
			Message:  "using ${VAR} substitution but env_file: is not configured",
			Severity: SeverityError,
		})
	}

	return violations
}

// ReadinessProbeRule checks that processes have readiness probes.
type ReadinessProbeRule struct{}

func (r ReadinessProbeRule) Name() string { return "readiness-probe" }
func (r ReadinessProbeRule) Description() string {
	return "Processes should have readiness probes for health checking"
}

func (r ReadinessProbeRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	for name, proc := range pc.Processes {
		if proc.Disabled {
			continue
		}

		if !proc.HasReadinessProbe() {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("process '%s' should have readiness_probe for health checking", name),
				Severity: SeverityWarning,
			})
		}
	}

	return violations
}

// ===== Fmt Rules (auto-fixable) =====

// SortProcessesRule sorts processes alphabetically.
type SortProcessesRule struct{}

func (r SortProcessesRule) Name() string        { return "sort-processes" }
func (r SortProcessesRule) Description() string { return "Processes should be sorted alphabetically" }

func (r SortProcessesRule) Check(pc *ProcessCompose) []Violation {
	// Get process names in order they appear in file
	var names []string
	inProcesses := false
	processIndent := ""

	for _, line := range pc.Lines {
		if strings.TrimSpace(line) == "processes:" {
			inProcesses = true
			continue
		}
		if inProcesses {
			// Detect first process to get indentation
			if processIndent == "" && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				// Get leading whitespace
				trimmed := strings.TrimLeft(line, " \t")
				processIndent = line[:len(line)-len(trimmed)]
			}

			// Check if this line is at process level
			if processIndent != "" && strings.HasPrefix(line, processIndent) {
				trimmed := strings.TrimSpace(line)
				if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, ":") {
					name := strings.TrimSuffix(trimmed, ":")
					names = append(names, name)
				} else if idx := strings.Index(trimmed, ":"); idx > 0 && !strings.ContainsAny(trimmed[:idx], " \t") {
					// Handle "name:" at beginning
					name := trimmed[:idx]
					names = append(names, name)
				}
			}
		}
	}

	// Check if sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			return []Violation{{
				File:     pc.Path,
				Line:     1,
				Rule:     r.Name(),
				Message:  "processes are not sorted alphabetically",
				Severity: SeverityWarning,
				Fixable:  true,
			}}
		}
	}

	return nil
}

func (r SortProcessesRule) Fix(pc *ProcessCompose) ([]byte, error) {
	// This is complex to implement correctly while preserving comments
	// For now, return content unchanged
	return pc.RawContent, nil
}

// VersionRule checks that version is specified.
type VersionRule struct{}

func (r VersionRule) Name() string        { return "version" }
func (r VersionRule) Description() string { return "Version should be specified" }

func (r VersionRule) Check(pc *ProcessCompose) []Violation {
	if pc.Version == "" {
		return []Violation{{
			File:     pc.Path,
			Line:     1,
			Rule:     r.Name(),
			Message:  "version: field is missing",
			Severity: SeverityError,
			Fixable:  true,
		}}
	}
	return nil
}

func (r VersionRule) Fix(pc *ProcessCompose) ([]byte, error) {
	if pc.Version != "" {
		return pc.RawContent, nil
	}

	// Prepend version
	content := "version: \"0.5\"\n\n" + string(pc.RawContent)

	// Remove duplicate if it was just missing the value
	re := regexp.MustCompile(`version:\s*\n`)
	content = re.ReplaceAllString(content, "")

	return []byte(content), nil
}

// ===== Rule Registry =====

// AllLintRules returns all lint rules.
func AllLintRules() []Rule {
	return []Rule{
		TaskCommandRule{},     // Commands should use task :run
		TaskHealthRule{},      // Health probes should use task :health
		ShutdownConfigRule{},  // Shutdown configuration
		RestartPolicyRule{},   // Restart policy
		EnvSubstitutionRule{}, // Secrets use ${VAR}
		EnvFileRule{},         // env_file configured if using substitution
		ReadinessProbeRule{},  // Readiness probes configured
	}
}

// AllFmtRules returns all auto-fixable fmt rules.
func AllFmtRules() []FmtRule {
	return []FmtRule{
		VersionRule{},       // Version field
		SortProcessesRule{}, // Alphabetical sorting
	}
}
