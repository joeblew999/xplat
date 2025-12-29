package manifest

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// ManifestFileName is the default manifest file name.
	ManifestFileName = "xplat.yaml"

	// DefaultAPIVersion is the current API version.
	DefaultAPIVersion = "xplat/v1"
)

// Loader loads manifests from local files or remote URLs.
type Loader struct {
	httpClient *http.Client
}

// NewLoader creates a new manifest loader.
func NewLoader() *Loader {
	return &Loader{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// LoadFile loads a manifest from a local file path.
func (l *Loader) LoadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}
	return l.parse(data, path)
}

// LoadDir loads a manifest from a directory (looks for xplat.yaml).
func (l *Loader) LoadDir(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	return l.LoadFile(path)
}

// LoadURL loads a manifest from a remote URL.
func (l *Loader) LoadURL(url string) (*Manifest, error) {
	resp, err := l.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest URL returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest response: %w", err)
	}

	return l.parse(data, url)
}

// LoadGitHub loads a manifest from a GitHub repository.
// repo format: "owner/repo" or "owner/repo@ref"
func (l *Loader) LoadGitHub(repo string) (*Manifest, error) {
	owner, repoName, ref := parseGitHubRepo(repo)
	if ref == "" {
		ref = "main"
	}

	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		owner, repoName, ref, ManifestFileName)

	return l.LoadURL(url)
}

// parse parses manifest YAML data.
func (l *Loader) parse(data []byte, source string) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest from %s: %w", source, err)
	}

	// Validate
	if err := l.validate(&m); err != nil {
		return nil, fmt.Errorf("invalid manifest from %s: %w", source, err)
	}

	return &m, nil
}

// validate checks that the manifest is valid.
func (l *Loader) validate(m *Manifest) error {
	if m.APIVersion != "" && m.APIVersion != DefaultAPIVersion {
		return fmt.Errorf("unsupported apiVersion: %s (expected %s)", m.APIVersion, DefaultAPIVersion)
	}

	if m.Name == "" {
		return fmt.Errorf("name is required")
	}

	if m.Version == "" {
		return fmt.Errorf("version is required")
	}

	return nil
}

// parseGitHubRepo parses "owner/repo" or "owner/repo@ref".
func parseGitHubRepo(repo string) (owner, name, ref string) {
	// Check for @ref suffix
	if idx := strings.LastIndex(repo, "@"); idx != -1 {
		ref = repo[idx+1:]
		repo = repo[:idx]
	}

	// Split owner/name
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		owner = parts[0]
		name = parts[1]
	}

	return
}

// Discover finds all xplat.yaml manifests in a directory tree.
func (l *Loader) Discover(root string, patterns []string) ([]*Manifest, error) {
	var manifests []*Manifest

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == ManifestFileName {
			m, err := l.LoadFile(path)
			if err != nil {
				// Log but continue
				fmt.Fprintf(os.Stderr, "warning: failed to load %s: %v\n", path, err)
				return nil
			}
			manifests = append(manifests, m)
		}

		return nil
	})

	return manifests, err
}

// DiscoverPlat finds manifests in plat-* directories under a root.
func (l *Loader) DiscoverPlat(root string) ([]*Manifest, error) {
	var manifests []*Manifest

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Look for plat-* directories
		if !strings.HasPrefix(entry.Name(), "plat-") {
			continue
		}

		manifestPath := filepath.Join(root, entry.Name(), ManifestFileName)
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		m, err := l.LoadFile(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load %s: %v\n", manifestPath, err)
			continue
		}

		manifests = append(manifests, m)
	}

	return manifests, nil
}
