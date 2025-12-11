package taskfile

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

// Violation represents a rule violation found in a Taskfile.
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

// Rule defines a validation rule for Taskfiles.
type Rule interface {
	Name() string
	Description() string
	Check(tf *Taskfile) []Violation
}

// FmtRule defines an auto-fixable formatting rule.
type FmtRule interface {
	Rule
	Fix(tf *Taskfile) ([]byte, error) // Returns modified content
}

// ===== Lint Rules =====

// ArchetypeVarsRule checks that required vars exist for the detected archetype.
type ArchetypeVarsRule struct{}

func (r ArchetypeVarsRule) Name() string        { return "archetype-vars" }
func (r ArchetypeVarsRule) Description() string { return "Check required vars for archetype" }

func (r ArchetypeVarsRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	info := DetectArchetype(tf)
	for _, suffix := range info.RequiredVars {
		if !tf.HasVar(suffix) {
			violations = append(violations, Violation{
				File:     tf.Path,
				Line:     tf.FindLineNumber("vars:"),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("archetype %s requires var with suffix %s", info.Type, suffix),
				Severity: SeverityError,
			})
		}
	}

	return violations
}

// ArchetypeTasksRule checks that required tasks exist for the detected archetype.
type ArchetypeTasksRule struct{}

func (r ArchetypeTasksRule) Name() string        { return "archetype-tasks" }
func (r ArchetypeTasksRule) Description() string { return "Check required tasks for archetype" }

func (r ArchetypeTasksRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	info := DetectArchetype(tf)
	for _, taskName := range info.RequiredTasks {
		if !tf.HasTask(taskName) {
			violations = append(violations, Violation{
				File:     tf.Path,
				Line:     tf.FindLineNumber("tasks:"),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("archetype %s requires task: %s", info.Type, taskName),
				Severity: SeverityError,
			})
		}
	}

	return violations
}

// CheckDepsStatusRule checks that check:deps has a status: section.
// Only applies to Tool archetype where binary installation check makes sense.
// External is excluded because they're often version checkers that should run every time.
// Bootstrap is excluded because it has special self-bootstrapping logic.
type CheckDepsStatusRule struct{}

func (r CheckDepsStatusRule) Name() string        { return "check-deps-status" }
func (r CheckDepsStatusRule) Description() string { return "check:deps should have status: for idempotency" }

func (r CheckDepsStatusRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Only check Tool archetype - these install binaries and should skip if present
	// External tools are often version checkers that should run every time
	// Builder/Aggregation/Bootstrap have different check:deps semantics
	info := DetectArchetype(tf)
	if info.Type != ArchetypeTool {
		return violations
	}

	task, ok := tf.GetTask("check:deps")
	if !ok {
		return violations // No check:deps task, archetype-tasks rule handles this
	}

	if len(task.Status) == 0 {
		violations = append(violations, Violation{
			File:     tf.Path,
			Line:     tf.FindLineNumber("check:deps:"),
			Rule:     r.Name(),
			Message:  "check:deps should have status: section for idempotency",
			Severity: SeverityError,
		})
	}

	return violations
}

// DocHeaderRule checks for documentation header comment.
type DocHeaderRule struct{}

func (r DocHeaderRule) Name() string        { return "doc-header" }
func (r DocHeaderRule) Description() string { return "Every taskfile should have a documentation header" }

func (r DocHeaderRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Check if file starts with a comment
	if len(tf.Lines) == 0 || !strings.HasPrefix(strings.TrimSpace(tf.Lines[0]), "#") {
		violations = append(violations, Violation{
			File:     tf.Path,
			Line:     1,
			Rule:     r.Name(),
			Message:  "missing documentation header comment",
			Severity: SeverityWarning,
		})
	}

	return violations
}

// ===== Fmt Rules (auto-fixable) =====

// ExeExtRule checks and fixes missing {{exeExt}} in _BIN vars.
type ExeExtRule struct{}

func (r ExeExtRule) Name() string        { return "exeext" }
func (r ExeExtRule) Description() string { return "_BIN vars must include {{exeExt}}" }

func (r ExeExtRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	for k, v := range tf.Vars {
		if !strings.HasSuffix(strings.ToUpper(k), "_BIN") {
			continue
		}
		val, ok := v.(string)
		if !ok {
			continue
		}
		if !strings.Contains(val, "{{exeExt}}") && !strings.Contains(val, ".exe") {
			violations = append(violations, Violation{
				File:     tf.Path,
				Line:     tf.FindLineNumber(k + ":"),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("%s is missing {{exeExt}}", k),
				Severity: SeverityError,
				Fixable:  true,
			})
		}
	}

	return violations
}

func (r ExeExtRule) Fix(tf *Taskfile) ([]byte, error) {
	content := string(tf.RawContent)

	// Pattern to match _BIN vars without {{exeExt}}
	// Example: DUMMY_BIN: 'dummy' -> DUMMY_BIN: 'dummy{{exeExt}}'
	re := regexp.MustCompile(`(_BIN:\s*['"])([^'"{}]+)(['"])`)
	content = re.ReplaceAllString(content, "${1}${2}{{exeExt}}${3}")

	return []byte(content), nil
}

// BareXplatRule checks and fixes bare 'xplat' commands.
type BareXplatRule struct{}

func (r BareXplatRule) Name() string        { return "bare-xplat" }
func (r BareXplatRule) Description() string { return "Use {{.XPLAT_BIN}} instead of bare xplat" }

func (r BareXplatRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Check each line for bare xplat usage
	for i, line := range tf.Lines {
		// Skip if it's already using the variable
		if strings.Contains(line, "{{.XPLAT_BIN}}") {
			continue
		}
		// Skip comments
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// Check for bare xplat command
		if strings.Contains(line, "- xplat ") || strings.Contains(line, "- 'xplat ") || strings.Contains(line, `- "xplat `) {
			violations = append(violations, Violation{
				File:     tf.Path,
				Line:     i + 1,
				Rule:     r.Name(),
				Message:  "use {{.XPLAT_BIN}} instead of bare xplat",
				Severity: SeverityError,
				Fixable:  true,
			})
		}
	}

	return violations
}

func (r BareXplatRule) Fix(tf *Taskfile) ([]byte, error) {
	content := string(tf.RawContent)

	// Replace bare xplat with {{.XPLAT_BIN}}
	// Patterns: - xplat, - 'xplat, - "xplat
	replacements := []struct {
		old, new string
	}{
		{"- xplat ", "- '{{.XPLAT_BIN}} "},
		{"- 'xplat ", "- '{{.XPLAT_BIN}} "},
		{`- "xplat `, `- "{{.XPLAT_BIN}} `},
	}

	for _, r := range replacements {
		content = strings.ReplaceAll(content, r.old, r.new)
	}

	return []byte(content), nil
}

// QuoteEchoRule checks and fixes unquoted echo statements with colons.
type QuoteEchoRule struct{}

func (r QuoteEchoRule) Name() string        { return "quote-echo" }
func (r QuoteEchoRule) Description() string { return "Quote echo statements containing colons" }

func (r QuoteEchoRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	for i, line := range tf.Lines {
		trimmed := strings.TrimSpace(line)
		// Check for unquoted echo with colon
		if strings.HasPrefix(trimmed, "- echo ") && strings.Contains(trimmed, ":") {
			// Not already quoted
			if !strings.HasPrefix(trimmed, "- 'echo") && !strings.HasPrefix(trimmed, `- "echo`) {
				violations = append(violations, Violation{
					File:     tf.Path,
					Line:     i + 1,
					Rule:     r.Name(),
					Message:  "echo statement with colon should be quoted",
					Severity: SeverityWarning,
					Fixable:  true,
				})
			}
		}
	}

	return violations
}

func (r QuoteEchoRule) Fix(tf *Taskfile) ([]byte, error) {
	lines := make([]string, len(tf.Lines))
	copy(lines, tf.Lines)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- echo ") && strings.Contains(trimmed, ":") {
			if !strings.HasPrefix(trimmed, "- 'echo") && !strings.HasPrefix(trimmed, `- "echo`) {
				// Get the indentation
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				// Quote the entire command
				echoContent := strings.TrimPrefix(trimmed, "- ")
				lines[i] = indent + "- '" + echoContent + "'"
			}
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// ===== Rule Registry =====

// AllLintRules returns all lint rules.
func AllLintRules() []Rule {
	return []Rule{
		ArchetypeVarsRule{},
		ArchetypeTasksRule{},
		CheckDepsStatusRule{},
		DocHeaderRule{},
	}
}

// AllFmtRules returns all auto-fixable fmt rules.
func AllFmtRules() []FmtRule {
	return []FmtRule{
		ExeExtRule{},
		BareXplatRule{},
		QuoteEchoRule{},
	}
}
