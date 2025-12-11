// Package taskfile provides Taskfile parsing and validation utilities.
package taskfile

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Taskfile represents a parsed Taskfile with all relevant sections.
type Taskfile struct {
	Path     string           // File path
	Version  string           `yaml:"version"`
	Includes map[string]any   `yaml:"includes"` // Can be string or map with taskfile/optional keys
	Vars     map[string]any   `yaml:"vars"`
	Tasks    map[string]Task  `yaml:"tasks"`

	// Parsed metadata
	RawContent []byte
	Lines      []string // For line number lookups
}

// Task represents a task definition.
type Task struct {
	Desc     string   `yaml:"desc"`
	Deps     []any    `yaml:"deps"`
	Cmds     []any    `yaml:"cmds"`
	Status   []string `yaml:"status"`
	Vars     map[string]any `yaml:"vars"`
	Requires *Requires `yaml:"requires"`
	Internal bool     `yaml:"internal"`
}

// Requires represents task requirements.
type Requires struct {
	Vars []string `yaml:"vars"`
}

// Parse reads and parses a Taskfile from the given path.
func Parse(path string) (*Taskfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tf Taskfile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, err
	}

	tf.Path = path
	tf.RawContent = data
	tf.Lines = strings.Split(string(data), "\n")

	return &tf, nil
}

// GetVarString returns a var value as string, handling templates.
func (tf *Taskfile) GetVarString(name string) string {
	if v, ok := tf.Vars[name]; ok {
		switch val := v.(type) {
		case string:
			return val
		case int:
			return ""
		case bool:
			return ""
		}
	}
	return ""
}

// HasVar checks if a variable with the given pattern exists.
// Pattern can be:
//   - Suffix: "_CGO" matches "DUMMY_CGO"
//   - Contains: "_BUILD_" matches "GO_BUILD_DIR"
func (tf *Taskfile) HasVar(pattern string) bool {
	pattern = strings.ToUpper(pattern)
	for k := range tf.Vars {
		upperK := strings.ToUpper(k)
		// Check both suffix and contains
		if strings.HasSuffix(upperK, pattern) || strings.Contains(upperK, pattern) {
			return true
		}
	}
	return false
}

// GetVarBySuffix returns the first var matching the suffix.
// For example, GetVarBySuffix("_BIN") might return "DUMMY_BIN", "dummy{{exeExt}}".
func (tf *Taskfile) GetVarBySuffix(suffix string) (name, value string, found bool) {
	for k, v := range tf.Vars {
		if strings.HasSuffix(strings.ToUpper(k), strings.ToUpper(suffix)) {
			switch val := v.(type) {
			case string:
				return k, val, true
			}
		}
	}
	return "", "", false
}

// HasVarValue checks if any variable matching the suffix has the given value.
// For example, HasVarValue("_CGO", "1") returns true if DUMMY_CGO='1'.
func (tf *Taskfile) HasVarValue(suffix, value string) bool {
	for k, v := range tf.Vars {
		if strings.HasSuffix(strings.ToUpper(k), strings.ToUpper(suffix)) {
			switch val := v.(type) {
			case string:
				// Handle quoted values like '1' or "1"
				cleanVal := strings.Trim(val, "'\"")
				if cleanVal == value {
					return true
				}
			}
		}
	}
	return false
}

// HasTask checks if a task exists.
func (tf *Taskfile) HasTask(name string) bool {
	_, ok := tf.Tasks[name]
	return ok
}

// GetTask returns a task by name.
func (tf *Taskfile) GetTask(name string) (Task, bool) {
	t, ok := tf.Tasks[name]
	return t, ok
}

// FindLineNumber finds the line number (1-indexed) of a pattern in the file.
func (tf *Taskfile) FindLineNumber(pattern string) int {
	for i, line := range tf.Lines {
		if strings.Contains(line, pattern) {
			return i + 1
		}
	}
	return 0
}

// FindTaskfiles recursively finds all Taskfiles in a directory.
func FindTaskfiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		// Match Taskfile patterns
		name := info.Name()
		if strings.HasPrefix(name, "Taskfile") && strings.HasSuffix(name, ".yml") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}
