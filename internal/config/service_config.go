// Package config contains service configuration file handling.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ServiceConfig holds global xplat service settings.
// Stored at ~/.xplat/service.yaml - configure once, runs consistently.
type ServiceConfig struct {
	// UI enables Task UI web interface
	UI bool `yaml:"ui,omitempty"`
	// UIPort is the port for Task UI (default: 8760)
	UIPort string `yaml:"ui_port,omitempty"`

	// MCP enables MCP HTTP server for AI IDE integration
	MCP bool `yaml:"mcp,omitempty"`
	// MCPPort is the port for MCP server (default: 8762)
	MCPPort string `yaml:"mcp_port,omitempty"`

	// Sync enables GitHub sync poller for Task cache invalidation
	Sync bool `yaml:"sync,omitempty"`
	// SyncRepos is comma-separated repos to poll (empty = auto-discover from Taskfile.yml)
	SyncRepos string `yaml:"sync_repos,omitempty"`
	// SyncInterval is poll interval (default: 5m)
	SyncInterval string `yaml:"sync_interval,omitempty"`
}

// LoadServiceConfig reads the service config from ~/.xplat/service.yaml.
// Returns default config (all features enabled) if file doesn't exist.
func LoadServiceConfig() (*ServiceConfig, error) {
	path := XplatServiceConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file = use defaults (all features enabled)
			return DefaultServiceConfig(), nil
		}
		return nil, err
	}

	// Start with defaults, then overlay file settings
	cfg := DefaultServiceConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveServiceConfig writes the service config to ~/.xplat/service.yaml.
func SaveServiceConfig(cfg *ServiceConfig) error {
	path := XplatServiceConfig()

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, DefaultDirPerms); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, DefaultFilePerms)
}

// XplatServiceConfig returns the path to ~/.xplat/service.yaml.
func XplatServiceConfig() string {
	return filepath.Join(XplatHome(), "service.yaml")
}

// DefaultServiceConfig returns the default service configuration.
// By default, UI, MCP, and Sync are all enabled.
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		UI:           true,
		UIPort:       DefaultUIPort,
		MCP:          true,
		MCPPort:      DefaultMCPPort,
		Sync:         true,
		SyncRepos:    "", // auto-discover from Taskfile.yml
		SyncInterval: DefaultSyncInterval,
	}
}

// ApplyDefaults fills in default values for unset fields.
// Features default to ON (UI, MCP, Sync all enabled).
func (c *ServiceConfig) ApplyDefaults() {
	if c.UIPort == "" {
		c.UIPort = DefaultUIPort
	}
	if c.MCPPort == "" {
		c.MCPPort = DefaultMCPPort
	}
	if c.SyncInterval == "" {
		c.SyncInterval = DefaultSyncInterval
	}
}
