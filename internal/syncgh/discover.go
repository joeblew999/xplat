package syncgh

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DiscoverReposFromTaskfile scans a Taskfile.yml for remote includes and
// extracts the GitHub repos to watch.
//
// Supported URL patterns:
//   - https://raw.githubusercontent.com/owner/repo/branch/path
//   - https://github.com/owner/repo.git//path
//   - https://github.com/owner/repo/raw/branch/path
//
// Returns a deduplicated list of "owner/repo" strings.
func DiscoverReposFromTaskfile(taskfilePath string) ([]string, error) {
	data, err := os.ReadFile(taskfilePath)
	if err != nil {
		return nil, err
	}

	return extractReposFromYAML(data)
}

// DiscoverReposFromProject scans all Taskfile.yml files in a project directory
// for remote includes and extracts the GitHub repos to watch.
func DiscoverReposFromProject(projectDir string) ([]string, error) {
	seen := make(map[string]bool)
	var repos []string

	// Search patterns for Taskfile locations
	patterns := []string{
		"Taskfile.yml",
		"Taskfile.yaml",
		"taskfiles/*.yml",
		"taskfiles/*.yaml",
		"**/Taskfile.yml",
		"**/Taskfile.yaml",
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(projectDir, pattern))
		if err != nil {
			continue
		}

		for _, match := range matches {
			discovered, err := DiscoverReposFromTaskfile(match)
			if err != nil {
				continue // Skip files that can't be parsed
			}

			for _, repo := range discovered {
				if !seen[repo] {
					seen[repo] = true
					repos = append(repos, repo)
				}
			}
		}
	}

	return repos, nil
}

// extractReposFromYAML parses YAML and extracts GitHub repos from remote taskfile URLs.
func extractReposFromYAML(data []byte) ([]string, error) {
	var taskfile struct {
		Includes map[string]interface{} `yaml:"includes"`
	}

	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var repos []string

	for _, include := range taskfile.Includes {
		var url string

		switch v := include.(type) {
		case string:
			url = v
		case map[string]interface{}:
			if tf, ok := v["taskfile"].(string); ok {
				url = tf
			}
		}

		if url == "" {
			continue
		}

		repo := extractRepoFromURL(url)
		if repo != "" && !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}

	return repos, nil
}

// Patterns to extract owner/repo from various GitHub URL formats
var (
	// https://raw.githubusercontent.com/owner/repo/branch/path
	rawGitHubPattern = regexp.MustCompile(`raw\.githubusercontent\.com/([^/]+)/([^/]+)/`)

	// https://github.com/owner/repo.git//path or https://github.com/owner/repo/...
	gitHubPattern = regexp.MustCompile(`github\.com/([^/]+)/([^/\.]+)`)
)

// extractRepoFromURL extracts "owner/repo" from a GitHub URL.
// Returns empty string if URL is not a GitHub URL.
func extractRepoFromURL(url string) string {
	// Skip local paths
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}

	// Try raw.githubusercontent.com pattern first
	if matches := rawGitHubPattern.FindStringSubmatch(url); len(matches) >= 3 {
		return matches[1] + "/" + matches[2]
	}

	// Try github.com pattern
	if matches := gitHubPattern.FindStringSubmatch(url); len(matches) >= 3 {
		repo := matches[2]
		// Remove .git suffix if present
		repo = strings.TrimSuffix(repo, ".git")
		return matches[1] + "/" + repo
	}

	return ""
}

// DiscoverReposToConfigs converts discovered repos to RepoConfig slice.
// All repos are configured to watch the "main" branch by default.
func DiscoverReposToConfigs(repos []string) []RepoConfig {
	configs := make([]RepoConfig, len(repos))
	for i, repo := range repos {
		configs[i] = RepoConfig{
			Subsystem: repo,
			Branch:    "main",
		}
	}
	return configs
}
