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
// These rules focus on cross-platform compatibility and idempotency,
// not on enforcing specific project structures or archetypes.

// CrossPlatformCmdsRule checks for direct shell commands that should use xplat os.
type CrossPlatformCmdsRule struct{}

func (r CrossPlatformCmdsRule) Name() string { return "cross-platform-cmds" }
func (r CrossPlatformCmdsRule) Description() string {
	return "Use xplat os commands for cross-platform compatibility"
}

func (r CrossPlatformCmdsRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Commands that should use xplat os instead
	// Maps command prefix to xplat os equivalent
	directCmds := map[string]string{
		"git clone":    "xplat os git clone",
		"git checkout": "xplat os git checkout",
		"git pull":     "xplat os git pull",
		"rm -rf":       "xplat os rm -rf",
		"rm -r":        "xplat os rm -r",
		"mkdir -p":     "xplat os mkdir -p",
		"cp -r":        "xplat os cp -r",
		"mv ":          "xplat os mv",
	}

	for i, line := range tf.Lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip lines that already use xplat os
		if strings.Contains(trimmed, "xplat os ") {
			continue
		}

		// Check for direct commands in task cmds
		for cmd, replacement := range directCmds {
			// Check various YAML patterns: - cmd, - 'cmd, - "cmd
			patterns := []string{
				"- " + cmd,
				"- '" + cmd,
				`- "` + cmd,
			}
			for _, pattern := range patterns {
				if strings.Contains(trimmed, pattern) {
					violations = append(violations, Violation{
						File:     tf.Path,
						Line:     i + 1,
						Rule:     r.Name(),
						Message:  fmt.Sprintf("use '%s' instead of '%s' for cross-platform compatibility", replacement, cmd),
						Severity: SeverityWarning,
						Fixable:  false, // Could be auto-fixed but risky
					})
					break
				}
			}
		}
	}

	return violations
}

// IdempotentDepsRule checks that deps:* tasks have status: sections.
// This applies universally - any task that installs/clones dependencies should be idempotent.
type IdempotentDepsRule struct{}

func (r IdempotentDepsRule) Name() string { return "idempotent-deps" }
func (r IdempotentDepsRule) Description() string {
	return "deps:* tasks should have status: sections for idempotency"
}

func (r IdempotentDepsRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Check all tasks that look like dependency installation
	depsTasks := []string{"deps:install", "deps:clone", "deps:download"}

	for _, taskName := range depsTasks {
		task, ok := tf.GetTask(taskName)
		if !ok {
			continue // Task doesn't exist, that's fine
		}

		// Skip aggregator tasks that only have deps: and no cmds:
		// Their idempotency is handled by the sub-tasks they depend on
		if len(task.Cmds) == 0 && len(task.Deps) > 0 {
			continue
		}

		if len(task.Status) == 0 {
			violations = append(violations, Violation{
				File:     tf.Path,
				Line:     tf.FindLineNumber(taskName + ":"),
				Rule:     r.Name(),
				Message:  fmt.Sprintf("%s should have status: section for idempotency", taskName),
				Severity: SeverityWarning,
			})
		}
	}

	return violations
}

// DocHeaderRule checks for documentation header comment.
type DocHeaderRule struct{}

func (r DocHeaderRule) Name() string { return "doc-header" }
func (r DocHeaderRule) Description() string {
	return "Every taskfile should have a documentation header"
}

func (r DocHeaderRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Check if file has a documentation comment in the first few lines
	// Allow version: to come first, but expect a # comment within first 5 lines
	hasDocComment := false
	checkLines := 5
	if len(tf.Lines) < checkLines {
		checkLines = len(tf.Lines)
	}

	for i := 0; i < checkLines; i++ {
		trimmed := strings.TrimSpace(tf.Lines[i])
		// Look for a substantial comment (not just # alone)
		if strings.HasPrefix(trimmed, "#") && len(trimmed) > 2 {
			hasDocComment = true
			break
		}
	}

	if !hasDocComment {
		violations = append(violations, Violation{
			File:     tf.Path,
			Line:     1,
			Rule:     r.Name(),
			Message:  "missing documentation header comment in first 5 lines",
			Severity: SeverityWarning,
		})
	}

	return violations
}

// DebugTaskRule checks that subsystem Taskfiles have a debug:self task.
// The debug:self task prints all vars for troubleshooting.
type DebugTaskRule struct{}

func (r DebugTaskRule) Name() string        { return "debug-task" }
func (r DebugTaskRule) Description() string { return "Subsystem taskfiles should have debug:self task" }

func (r DebugTaskRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	// Skip root Taskfiles (they have includes:) - they use debug:all pattern
	if len(tf.Includes) > 0 {
		return violations
	}

	// Skip if no vars defined (probably not a real subsystem)
	if len(tf.Vars) == 0 {
		return violations
	}

	// Check for debug:self task
	_, hasDebugSelf := tf.GetTask("debug:self")
	_, hasDebug := tf.GetTask("debug")

	if !hasDebugSelf && !hasDebug {
		violations = append(violations, Violation{
			File:     tf.Path,
			Line:     1,
			Rule:     r.Name(),
			Message:  "missing debug:self task for printing vars",
			Severity: SeverityWarning,
		})
	}

	return violations
}

// ===== Fmt Rules (auto-fixable) =====

// CrossPlatformCmdsFmtRule auto-fixes shell commands to use xplat os equivalents.
type CrossPlatformCmdsFmtRule struct{}

func (r CrossPlatformCmdsFmtRule) Name() string { return "cross-platform-cmds" }
func (r CrossPlatformCmdsFmtRule) Description() string {
	return "Auto-fix shell commands to use xplat os for cross-platform compatibility"
}

func (r CrossPlatformCmdsFmtRule) Check(tf *Taskfile) []Violation {
	// FmtRules don't report violations - the lint rule does that
	// This fmt rule only provides the Fix() method
	return nil
}

func (r CrossPlatformCmdsFmtRule) Fix(tf *Taskfile) ([]byte, error) {
	lines := make([]string, len(tf.Lines))
	copy(lines, tf.Lines)

	// Commands to replace: direct command -> xplat os command
	replacements := []struct {
		from string
		to   string
	}{
		{"rm -rf ", "xplat os rm -rf "},
		{"rm -r ", "xplat os rm -r "},
		{"mkdir -p ", "xplat os mkdir -p "},
		{"cp -r ", "xplat os cp -r "},
		{"git clone ", "xplat os git clone "},
		{"git checkout ", "xplat os git checkout "},
		{"git pull ", "xplat os git pull "},
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip lines that already use xplat os
		if strings.Contains(line, "xplat os ") {
			continue
		}

		// Only process command lines (start with -)
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}

		for _, rep := range replacements {
			// Handle various YAML patterns
			patterns := []string{
				"- " + rep.from,
				"- '" + rep.from,
				`- "` + rep.from,
			}
			for _, pattern := range patterns {
				if strings.Contains(line, pattern) {
					lines[i] = strings.Replace(line, rep.from, rep.to, 1)
					break
				}
			}
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// ExeExtRule checks and fixes missing {{exeExt}} in binary path vars.
// Convention: *_BIN = directory, *_BIN_PATH or *_BIN_NAME = actual binary
type ExeExtRule struct{}

func (r ExeExtRule) Name() string        { return "exeext" }
func (r ExeExtRule) Description() string { return "Binary path vars must include {{exeExt}}" }

func (r ExeExtRule) Check(tf *Taskfile) []Violation {
	var violations []Violation

	for k, v := range tf.Vars {
		upperKey := strings.ToUpper(k)

		// Only check _BIN_PATH vars (actual binary paths)
		// Skip _BIN (directories) and _BIN_NAME (just the name, no path)
		if !strings.HasSuffix(upperKey, "_BIN_PATH") {
			continue
		}

		val, ok := v.(string)
		if !ok {
			continue
		}

		// Skip if already has {{exeExt}} or .exe
		if strings.Contains(val, "{{exeExt}}") || strings.Contains(val, ".exe") {
			continue
		}

		// Skip if value looks like a directory (ends with / or references _BIN})
		if strings.HasSuffix(val, "/") {
			continue
		}

		violations = append(violations, Violation{
			File:     tf.Path,
			Line:     tf.FindLineNumber(k + ":"),
			Rule:     r.Name(),
			Message:  fmt.Sprintf("%s should include {{exeExt}} for Windows compatibility", k),
			Severity: SeverityWarning, // Downgrade to warning - not all binaries need .exe
			Fixable:  true,
		})
	}

	return violations
}

func (r ExeExtRule) Fix(tf *Taskfile) ([]byte, error) {
	content := string(tf.RawContent)

	// Pattern to match _BIN_PATH vars without {{exeExt}}
	// Example: NATS_BIN_PATH: '{{.NATS_BIN}}/nats-server' -> add {{exeExt}} before closing quote
	re := regexp.MustCompile(`(_BIN_PATH:\s*['"])([^'"]+)(['"])`)
	content = re.ReplaceAllStringFunc(content, func(match string) string {
		// Don't add if already has exeExt or .exe
		if strings.Contains(match, "{{exeExt}}") || strings.Contains(match, ".exe") {
			return match
		}
		// Insert {{exeExt}} before closing quote
		return re.ReplaceAllString(match, "${1}${2}{{exeExt}}${3}")
	})

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
// Rules focus on cross-platform compatibility and idempotency.
func AllLintRules() []Rule {
	return []Rule{
		CrossPlatformCmdsRule{}, // Use xplat os commands
		IdempotentDepsRule{},    // deps:* tasks need status:
		DocHeaderRule{},         // Documentation header
		DebugTaskRule{},         // Subsystem debug:self task
	}
}

// AllFmtRules returns all auto-fixable fmt rules.
func AllFmtRules() []FmtRule {
	return []FmtRule{
		CrossPlatformCmdsFmtRule{},
		ExeExtRule{},
		QuoteEchoRule{},
	}
}
