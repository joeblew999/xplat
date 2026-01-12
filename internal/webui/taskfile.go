package web

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Taskfile represents a parsed Taskfile.yml.
type Taskfile struct {
	Version  string             `yaml:"version"`
	Includes map[string]Include `yaml:"includes"`
	Tasks    map[string]Task    `yaml:"tasks"`
}

// Include represents a taskfile include directive.
type Include struct {
	Taskfile string `yaml:"taskfile"`
	Dir      string `yaml:"dir,omitempty"`
	Optional bool   `yaml:"optional,omitempty"`
}

// UnmarshalYAML handles both string and object forms of includes.
func (i *Include) UnmarshalYAML(value *yaml.Node) error {
	// Try simple string form first: "includes: foo: ./path.yml"
	if value.Kind == yaml.ScalarNode {
		i.Taskfile = value.Value
		return nil
	}
	// Object form: "includes: foo: { taskfile: ./path.yml, dir: ... }"
	type includeAlias Include
	var alias includeAlias
	if err := value.Decode(&alias); err != nil {
		return err
	}
	*i = Include(alias)
	return nil
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

// remoteTaskfileCache caches fetched remote taskfiles.
var remoteTaskfileCache = make(map[string][]byte)

// parseTaskfile parses a Taskfile.yml and returns the task definitions.
// It also resolves includes, fetching remote taskfiles as needed.
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

	// Process includes
	if len(tf.Includes) > 0 {
		if tf.Tasks == nil {
			tf.Tasks = make(map[string]Task)
		}
		for namespace, include := range tf.Includes {
			includedTf, err := resolveInclude(include, workDir)
			if err != nil {
				if include.Optional {
					continue
				}
				// Log but don't fail - allow UI to show local tasks
				fmt.Printf("Warning: Failed to load include '%s': %v\n", namespace, err)
				continue
			}
			// Add included tasks with namespace prefix
			for taskName, task := range includedTf.Tasks {
				if task.Internal {
					continue
				}
				prefixedName := namespace + ":" + taskName
				tf.Tasks[prefixedName] = task
			}
		}
	}

	return &tf, nil
}

// resolveInclude loads a taskfile from a local path or remote URL.
func resolveInclude(include Include, workDir string) (*Taskfile, error) {
	taskfilePath := include.Taskfile

	// Check if it's a remote URL
	if strings.HasPrefix(taskfilePath, "http://") || strings.HasPrefix(taskfilePath, "https://") {
		return fetchRemoteTaskfile(taskfilePath)
	}

	// Local path - resolve relative to workDir
	if !filepath.IsAbs(taskfilePath) {
		taskfilePath = filepath.Join(workDir, taskfilePath)
	}

	data, err := os.ReadFile(taskfilePath)
	if err != nil {
		return nil, err
	}

	var tf Taskfile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, err
	}

	return &tf, nil
}

// fetchRemoteTaskfile downloads and parses a remote taskfile.
func fetchRemoteTaskfile(url string) (*Taskfile, error) {
	// Check cache first
	if data, ok := remoteTaskfileCache[url]; ok {
		var tf Taskfile
		if err := yaml.Unmarshal(data, &tf); err != nil {
			return nil, err
		}
		return &tf, nil
	}

	// Check disk cache
	cacheDir := getCacheDir()
	cacheFile := filepath.Join(cacheDir, hashURL(url)+".yml")

	// Use cached file if it exists and is less than 1 hour old
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < time.Hour {
			data, err := os.ReadFile(cacheFile)
			if err == nil {
				remoteTaskfileCache[url] = data
				var tf Taskfile
				if err := yaml.Unmarshal(data, &tf); err != nil {
					return nil, err
				}
				return &tf, nil
			}
		}
	}

	// Fetch from remote
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote taskfile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch remote taskfile: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote taskfile: %w", err)
	}

	// Cache to memory and disk
	remoteTaskfileCache[url] = data
	_ = os.MkdirAll(cacheDir, 0755)
	_ = os.WriteFile(cacheFile, data, 0644)

	var tf Taskfile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("failed to parse remote taskfile: %w", err)
	}

	return &tf, nil
}

// getCacheDir returns the cache directory for remote taskfiles.
func getCacheDir() string {
	if cacheDir := os.Getenv("XDG_CACHE_HOME"); cacheDir != "" {
		return filepath.Join(cacheDir, "xplat", "taskfiles")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".cache", "xplat", "taskfiles")
}

// hashURL creates a short hash of a URL for cache filename.
func hashURL(url string) string {
	h := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", h[:8])
}
