package taskfile

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Include represents a taskfile include entry.
type Include struct {
	Name     string // Namespace (e.g., "mailerlite")
	Taskfile string // URL or path to taskfile
}

// AddInclude adds a remote taskfile include to a Taskfile.
// It modifies the file in place, preserving existing content and formatting.
func AddInclude(taskfilePath string, include Include) error {
	data, err := os.ReadFile(taskfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create minimal Taskfile with the include
			content := createMinimalTaskfile(include)
			return os.WriteFile(taskfilePath, []byte(content), 0644)
		}
		return fmt.Errorf("failed to read taskfile: %w", err)
	}

	content := string(data)

	// Check if include already exists
	if strings.Contains(content, include.Name+":") && strings.Contains(content, include.Taskfile) {
		return fmt.Errorf("include %q already exists in %s", include.Name, taskfilePath)
	}

	// Find or create includes section
	modified, err := addIncludeToContent(content, include)
	if err != nil {
		return err
	}

	// Write back
	return os.WriteFile(taskfilePath, []byte(modified), 0644)
}

// RemoveInclude removes a remote taskfile include from a Taskfile.
func RemoveInclude(taskfilePath string, namespace string) error {
	data, err := os.ReadFile(taskfilePath)
	if err != nil {
		return fmt.Errorf("failed to read taskfile: %w", err)
	}

	content := string(data)
	modified := removeIncludeFromContent(content, namespace)

	return os.WriteFile(taskfilePath, []byte(modified), 0644)
}

// HasInclude checks if a namespace is already included.
func HasInclude(taskfilePath string, namespace string) (bool, error) {
	data, err := os.ReadFile(taskfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Simple check - look for "namespace:" in includes section
	content := string(data)

	// Find includes section
	includesMatch := regexp.MustCompile(`(?m)^includes:\s*\n`).FindStringIndex(content)
	if includesMatch == nil {
		return false, nil
	}

	// Check if namespace exists after includes:
	afterIncludes := content[includesMatch[1]:]
	namespacePattern := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(namespace) + `:\s*$|^\s+` + regexp.QuoteMeta(namespace) + `:\s*\n`)
	return namespacePattern.MatchString(afterIncludes), nil
}

func createMinimalTaskfile(include Include) string {
	return fmt.Sprintf(`version: '3'

includes:
  %s:
    taskfile: %s

tasks:
  default:
    desc: List available tasks
    cmds:
      - task --list
`, include.Name, include.Taskfile)
}

func addIncludeToContent(content string, include Include) (string, error) {
	// Pattern to find includes section
	includesPattern := regexp.MustCompile(`(?m)^includes:\s*\n`)

	includeEntry := fmt.Sprintf("  %s:\n    taskfile: %s\n", include.Name, include.Taskfile)

	if match := includesPattern.FindStringIndex(content); match != nil {
		// Found includes section - add after it
		insertPos := match[1]
		return content[:insertPos] + includeEntry + content[insertPos:], nil
	}

	// No includes section - need to add one
	// Try to add after version line
	versionPattern := regexp.MustCompile(`(?m)^version:\s*['"]?\d['"]?\s*\n`)
	if match := versionPattern.FindStringIndex(content); match != nil {
		insertPos := match[1]
		newIncludes := "\nincludes:\n" + includeEntry
		return content[:insertPos] + newIncludes + content[insertPos:], nil
	}

	// No version line - add at beginning
	return "version: '3'\n\nincludes:\n" + includeEntry + "\n" + content, nil
}

func removeIncludeFromContent(content string, namespace string) string {
	// This is a simplified removal - handles the common case
	// Pattern: "  namespace:\n    taskfile: ..."
	pattern := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(namespace) + `:\s*\n\s+taskfile:\s*[^\n]+\n`)
	return pattern.ReplaceAllString(content, "")
}
