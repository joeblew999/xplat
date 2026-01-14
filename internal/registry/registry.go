package registry

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

// Index URL points to the raw index.yaml in the xplat repo.
// This is the central registry that maps package names to repo URLs.
const DefaultIndexURL = "https://raw.githubusercontent.com/joeblew999/xplat/main/registry/index.yaml"

// Environment variable to override index URL (for local testing).
const EnvIndexURL = "XPLAT_INDEX_URL"

// Index represents the central package index (name -> repo mapping).
type Index struct {
	Packages map[string]IndexEntry `yaml:"packages"`
}

// IndexEntry is a single entry in the index.
type IndexEntry struct {
	Name        string `yaml:"-"`           // Package name (set during list)
	Repo        string `yaml:"repo"`        // e.g., "github.com/litesql/ha"
	Description string `yaml:"description"` // Short description
}

// Client provides access to the package registry using the hybrid approach:
// 1. Central index maps package names to repo URLs
// 2. Each repo's xplat.yaml provides full package metadata
type Client struct {
	indexURL   string
	httpClient *http.Client
	indexCache *Index
}

// NewClient creates a new registry client.
func NewClient() *Client {
	url := DefaultIndexURL
	if envURL := os.Getenv(EnvIndexURL); envURL != "" {
		url = envURL
	}
	return &Client{
		indexURL: url,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithIndexURL sets a custom index URL (for testing).
func (c *Client) WithIndexURL(url string) *Client {
	c.indexURL = url
	return c
}

// FetchIndex downloads and parses the central index.
func (c *Client) FetchIndex() (*Index, error) {
	if c.indexCache != nil {
		return c.indexCache, nil
	}

	resp, err := c.httpClient.Get(c.indexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index returned HTTP %d", resp.StatusCode)
	}

	var index Index
	if err := yaml.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	c.indexCache = &index
	return &index, nil
}

// LookupRepo returns the repo URL for a package name.
// Returns the name itself if it looks like a direct repo URL (contains "/").
func (c *Client) LookupRepo(name string) (string, error) {
	// If name contains "/", treat it as a direct repo URL
	if strings.Contains(name, "/") {
		return name, nil
	}

	index, err := c.FetchIndex()
	if err != nil {
		return "", err
	}

	entry, ok := index.Packages[name]
	if !ok {
		return "", fmt.Errorf("package %q not found in index", name)
	}

	return entry.Repo, nil
}

// FetchManifest fetches the xplat.yaml from a repo.
// repo should be like "github.com/litesql/ha"
func (c *Client) FetchManifest(repo string) (*Package, error) {
	// Build raw URL to xplat.yaml
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/xplat.yaml",
		strings.TrimPrefix(repo, "github.com/"))

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest from %s: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no xplat.yaml found in %s", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest fetch returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var pkg Package
	if err := yaml.Unmarshal(body, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Set repo URL if not already set
	if pkg.RepoURL == "" {
		pkg.RepoURL = "https://" + repo
	}

	return &pkg, nil
}

// GetPackage resolves a package name and fetches its full metadata.
// name can be either:
//   - A short name from the index (e.g., "litesql-ha")
//   - A full repo path (e.g., "github.com/litesql/ha")
func (c *Client) GetPackage(name string) (*Package, error) {
	repo, err := c.LookupRepo(name)
	if err != nil {
		return nil, err
	}

	pkg, err := c.FetchManifest(repo)
	if err != nil {
		return nil, err
	}

	// Use the lookup name if package name not set
	if pkg.Name == "" {
		pkg.Name = filepath.Base(name)
	}

	return pkg, nil
}

// ListPackages returns all packages from the index with basic info.
// Full metadata requires calling GetPackage for each.
func (c *Client) ListPackages() ([]IndexEntry, error) {
	index, err := c.FetchIndex()
	if err != nil {
		return nil, err
	}

	entries := make([]IndexEntry, 0, len(index.Packages))
	for name, entry := range index.Packages {
		entry.Name = name // Set the name from the map key
		entries = append(entries, entry)
	}

	return entries, nil
}

// LoadLocalIndex loads the index from a local file (for embedded/offline use).
func LoadLocalIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read local index: %w", err)
	}

	var index Index
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse local index: %w", err)
	}

	return &index, nil
}
