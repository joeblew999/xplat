package processcompose

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
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

		// Skip scheduled processes - they run periodically, not continuously
		if proc.Schedule != nil {
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

// ScheduleConfigRule validates scheduled process configuration (v1.87.0+).
type ScheduleConfigRule struct{}

func (r ScheduleConfigRule) Name() string { return "schedule-config" }
func (r ScheduleConfigRule) Description() string {
	return "Validates scheduled process configuration (cron/interval)"
}

func (r ScheduleConfigRule) Check(pc *ProcessCompose) []Violation {
	var violations []Violation

	for name, proc := range pc.Processes {
		if proc.Disabled || proc.Schedule == nil {
			continue
		}

		s := proc.Schedule

		// Check that either cron or interval is specified (not both, not neither)
		hasCron := s.Cron != ""
		hasInterval := s.Interval != ""

		if !hasCron && !hasInterval {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("scheduled process '%s' must have either 'cron' or 'interval' configured", name),
				Severity: SeverityError,
			})
		}

		if hasCron && hasInterval {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("scheduled process '%s' has both 'cron' and 'interval' - use only one", name),
				Severity: SeverityError,
			})
		}

		// Validate cron expression format with detailed field validation
		if hasCron {
			if cronErr := validateCronExpression(s.Cron); cronErr != "" {
				violations = append(violations, Violation{
					File:     pc.Path,
					Line:     pc.FindProcessLineNumber(name),
					Rule:     r.Name(),
					Message:  fmt.Sprintf("scheduled process '%s' cron '%s': %s", name, s.Cron, cronErr),
					Severity: SeverityError,
				})
			}
		}

		// Validate interval format (Go duration)
		if hasInterval {
			if !isValidDuration(s.Interval) {
				violations = append(violations, Violation{
					File:     pc.Path,
					Line:     pc.FindProcessLineNumber(name),
					Rule:     r.Name(),
					Message:  fmt.Sprintf("scheduled process '%s' interval '%s' is not a valid Go duration (e.g., \"30s\", \"5m\", \"1h\")", name, s.Interval),
					Severity: SeverityError,
				})
			}
		}

		// Warn if max_concurrent > 1 without explicit acknowledgment
		if s.MaxConcurrent > 1 {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("scheduled process '%s' allows %d concurrent executions - ensure this is intentional", name, s.MaxConcurrent),
				Severity: SeverityWarning,
			})
		}

		// Scheduled processes shouldn't have readiness probes (they're not long-running)
		if proc.HasReadinessProbe() {
			violations = append(violations, Violation{
				File:     pc.Path,
				Line:     pc.FindProcessLineNumber(name),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("scheduled process '%s' has readiness_probe but scheduled processes are not long-running", name),
				Severity: SeverityWarning,
			})
		}

		// Scheduled processes shouldn't have depends_on with process_healthy
		// (they run independently on schedule)
		for dep, cfg := range proc.DependsOn {
			if cfg.Condition == "process_healthy" {
				violations = append(violations, Violation{
					File:     pc.Path,
					Line:     pc.FindProcessLineNumber(name),
					Rule:     r.Name(),
					Message:  fmt.Sprintf("scheduled process '%s' depends on '%s' with process_healthy - consider using process_started or removing dependency", name, dep),
					Severity: SeverityWarning,
				})
			}
		}
	}

	return violations
}

// isValidDuration checks if s is a valid Go duration string using time.ParseDuration.
func isValidDuration(s string) bool {
	if s == "" {
		return false
	}
	_, err := time.ParseDuration(s)
	return err == nil
}

// validateCronExpression performs detailed validation of a cron expression.
// Returns an error message if invalid, empty string if valid.
func validateCronExpression(cron string) string {
	fields := strings.Fields(cron)
	if len(fields) != 5 {
		return fmt.Sprintf("should have 5 fields (minute hour day month weekday), got %d", len(fields))
	}

	fieldNames := []string{"minute", "hour", "day", "month", "weekday"}
	fieldRanges := []struct{ min, max int }{
		{0, 59},  // minute
		{0, 23},  // hour
		{1, 31},  // day
		{1, 12},  // month
		{0, 7},   // weekday (0 and 7 are both Sunday)
	}

	for i, field := range fields {
		if err := validateCronField(field, fieldNames[i], fieldRanges[i].min, fieldRanges[i].max); err != "" {
			return err
		}
	}

	return ""
}

// validateCronField validates a single cron field.
func validateCronField(field, name string, min, max int) string {
	// Handle wildcard
	if field == "*" {
		return ""
	}

	// Handle step values: */5, 0-30/5
	if strings.Contains(field, "/") {
		parts := strings.SplitN(field, "/", 2)
		if len(parts) != 2 {
			return fmt.Sprintf("%s: invalid step syntax '%s'", name, field)
		}
		// Validate the base part (before /)
		if parts[0] != "*" {
			if err := validateCronField(parts[0], name, min, max); err != "" {
				return err
			}
		}
		// Validate step value
		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return fmt.Sprintf("%s: step value must be positive integer, got '%s'", name, parts[1])
		}
		return ""
	}

	// Handle ranges: 1-5, 0-30
	if strings.Contains(field, "-") {
		parts := strings.SplitN(field, "-", 2)
		if len(parts) != 2 {
			return fmt.Sprintf("%s: invalid range syntax '%s'", name, field)
		}
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return fmt.Sprintf("%s: range values must be integers in '%s'", name, field)
		}
		if start < min || start > max {
			return fmt.Sprintf("%s: start value %d out of range (%d-%d)", name, start, min, max)
		}
		if end < min || end > max {
			return fmt.Sprintf("%s: end value %d out of range (%d-%d)", name, end, min, max)
		}
		if start > end {
			return fmt.Sprintf("%s: range start %d > end %d", name, start, end)
		}
		return ""
	}

	// Handle lists: 1,3,5
	if strings.Contains(field, ",") {
		for _, part := range strings.Split(field, ",") {
			if err := validateCronField(strings.TrimSpace(part), name, min, max); err != "" {
				return err
			}
		}
		return ""
	}

	// Plain number
	val, err := strconv.Atoi(field)
	if err != nil {
		return fmt.Sprintf("%s: '%s' is not a valid value", name, field)
	}
	if val < min || val > max {
		return fmt.Sprintf("%s: value %d out of range (%d-%d)", name, val, min, max)
	}

	return ""
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
		ScheduleConfigRule{},  // Scheduled process validation (v1.87.0+)
	}
}

// AllFmtRules returns all auto-fixable fmt rules.
func AllFmtRules() []FmtRule {
	return []FmtRule{
		VersionRule{},       // Version field
		SortProcessesRule{}, // Alphabetical sorting
	}
}
