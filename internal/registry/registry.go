package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DefaultRegistryURL is the default URL for the package registry.
const DefaultRegistryURL = "https://www.ubuntusoftware.net/pkg/registry.json"

// Client provides access to the package registry.
type Client struct {
	registryURL string
	httpClient  *http.Client
	cache       *Registry
}

// NewClient creates a new registry client.
func NewClient() *Client {
	return &Client{
		registryURL: DefaultRegistryURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithURL sets a custom registry URL (for testing).
func (c *Client) WithURL(url string) *Client {
	c.registryURL = url
	return c
}

// Fetch downloads and parses the registry.
func (c *Client) Fetch() (*Registry, error) {
	if c.cache != nil {
		return c.cache, nil
	}

	resp, err := c.httpClient.Get(c.registryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}

	var registry Registry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	c.cache = &registry
	return &registry, nil
}

// GetPackage returns a package by name.
func (c *Client) GetPackage(name string) (*Package, error) {
	reg, err := c.Fetch()
	if err != nil {
		return nil, err
	}

	pkg, ok := reg.Packages[name]
	if !ok {
		return nil, fmt.Errorf("package %q not found in registry", name)
	}

	return &pkg, nil
}

// ListPackages returns all packages in the registry.
func (c *Client) ListPackages() ([]Package, error) {
	reg, err := c.Fetch()
	if err != nil {
		return nil, err
	}

	packages := make([]Package, 0, len(reg.Packages))
	for _, pkg := range reg.Packages {
		packages = append(packages, pkg)
	}

	return packages, nil
}
