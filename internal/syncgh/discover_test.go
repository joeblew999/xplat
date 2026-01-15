package syncgh

import (
	"testing"
)

func TestExtractRepoFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		// raw.githubusercontent.com URLs
		{
			url:      "https://raw.githubusercontent.com/joeblew999/xplat/main/taskfiles/Taskfile.service.yml",
			expected: "joeblew999/xplat",
		},
		{
			url:      "https://raw.githubusercontent.com/go-task/task/v3/Taskfile.yml",
			expected: "go-task/task",
		},

		// github.com URLs with .git
		{
			url:      "https://github.com/joeblew999/xplat.git//taskfiles/Taskfile.plat.yml",
			expected: "joeblew999/xplat",
		},

		// github.com URLs without .git
		{
			url:      "https://github.com/owner/repo/raw/main/Taskfile.yml",
			expected: "owner/repo",
		},

		// Local paths (should return empty)
		{
			url:      "./tools/Taskfile.git.yml",
			expected: "",
		},
		{
			url:      "../shared/Taskfile.yml",
			expected: "",
		},

		// Non-GitHub URLs (should return empty)
		{
			url:      "https://gitlab.com/owner/repo/raw/main/Taskfile.yml",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractRepoFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("extractRepoFromURL(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestExtractReposFromYAML(t *testing.T) {
	yaml := `
version: '3'

includes:
  # String format
  remote: https://raw.githubusercontent.com/joeblew999/xplat/main/Taskfile.yml

  # Map format
  tools:
    taskfile: https://raw.githubusercontent.com/go-task/task/main/Taskfile.yml
    dir: ./tools

  # Local include (should be ignored)
  local:
    taskfile: ./local/Taskfile.yml

  # Another remote
  other: https://github.com/owner/repo.git//Taskfile.yml
`

	repos, err := extractReposFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("extractReposFromYAML failed: %v", err)
	}

	// Should find 3 unique repos
	if len(repos) != 3 {
		t.Errorf("expected 3 repos, got %d: %v", len(repos), repos)
	}

	// Check that expected repos are present
	expected := map[string]bool{
		"joeblew999/xplat": true,
		"go-task/task":     true,
		"owner/repo":       true,
	}

	for _, repo := range repos {
		if !expected[repo] {
			t.Errorf("unexpected repo: %s", repo)
		}
		delete(expected, repo)
	}

	if len(expected) > 0 {
		t.Errorf("missing repos: %v", expected)
	}
}

func TestDiscoverReposToConfigs(t *testing.T) {
	repos := []string{"joeblew999/xplat", "go-task/task"}
	configs := DiscoverReposToConfigs(repos)

	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}

	for i, cfg := range configs {
		if cfg.Subsystem != repos[i] {
			t.Errorf("config[%d].Subsystem = %q, want %q", i, cfg.Subsystem, repos[i])
		}
		if cfg.Branch != "main" {
			t.Errorf("config[%d].Branch = %q, want %q", i, cfg.Branch, "main")
		}
	}
}
