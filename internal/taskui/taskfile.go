package taskui

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Taskfile represents a parsed Taskfile.yml.
type Taskfile struct {
	Version string          `yaml:"version"`
	Tasks   map[string]Task `yaml:"tasks"`
}

// Task represents a task in a Taskfile.
type Task struct {
	Desc        string   `yaml:"desc"`
	Summary     string   `yaml:"summary"`
	Cmds        []any    `yaml:"cmds"`
	Deps        []any    `yaml:"deps"`
	Internal    bool     `yaml:"internal"`
	Interactive bool     `yaml:"interactive"`
}

// parseTaskfile parses a Taskfile.yml and returns the task definitions.
func parseTaskfile(filename, workDir string) (*Taskfile, error) {
	path := filename
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, filename)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tf Taskfile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, err
	}

	return &tf, nil
}
