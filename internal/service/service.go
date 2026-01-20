// Package service provides cross-platform system service management using kardianos/service.
package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/projects"
	"github.com/joeblew999/xplat/internal/updater"
	"github.com/kardianos/service"
)

const (
	updateInterval = 1 * time.Hour // Check for updates every hour
)

// Config holds service configuration.
type Config struct {
	Name        string // Service name (e.g., "xplat" or "xplat-myproject")
	DisplayName string // Human-readable name
	Description string // Service description
	WorkDir     string // Working directory for the service
	UserService bool   // Install as user service (not root)
	AutoUpdate  bool   // Enable automatic updates (default: true)
	Version     string // Current version (injected at build time)
	WithUI      bool   // Start Task UI alongside process-compose
	UIPort      string // Port for Task UI (default: "3000")
	WithMCP     bool   // Start MCP HTTP server alongside process-compose
	MCPPort     string // Port for MCP server (default: "8765")
	WithSync    bool   // Start GitHub sync poller for Task cache invalidation
	SyncRepos   string // Comma-separated list of repos to poll (e.g., "owner/repo,owner2/repo2")
	SyncInterval string // Poll interval (e.g., "5m", "1h")
}

// DefaultConfig returns the default service configuration.
func DefaultConfig() Config {
	workDir, _ := os.Getwd()
	return Config{
		Name:        "xplat",
		DisplayName: "xplat Service",
		Description: "xplat process orchestration service",
		WorkDir:     workDir,
		UserService: true,
		AutoUpdate:  true,
	}
}

// ConfigForProject returns a config named after the project directory.
func ConfigForProject(projectDir string) Config {
	name := filepath.Base(projectDir)
	return Config{
		Name:        fmt.Sprintf("xplat-%s", name),
		DisplayName: fmt.Sprintf("xplat: %s", name),
		Description: fmt.Sprintf("xplat service for %s", name),
		WorkDir:     projectDir,
		UserService: true,
		AutoUpdate:  true,
	}
}

// program implements the service.Interface.
type program struct {
	cmd        *exec.Cmd
	uiCmd      *exec.Cmd  // Task UI process (if WithUI is enabled)
	mcpCmd     *exec.Cmd  // MCP HTTP server process (if WithMCP is enabled)
	syncCmd    *exec.Cmd  // GitHub sync poller (if WithSync is enabled)
	workDir    string
	xplatBin   string
	autoUpdate bool
	version    string
	withUI     bool
	uiPort     string
	withMCP    bool
	mcpPort    string
	withSync   bool
	syncRepos  string
	syncInterval string
	stopChan   chan struct{}
}

func (p *program) Start(s service.Service) error {
	log.Printf("Starting %s service...", s.String())
	p.stopChan = make(chan struct{})
	go p.run()
	if p.withUI {
		go p.runUI()
	}
	if p.withMCP {
		go p.runMCP()
	}
	if p.withSync {
		go p.runSync()
	}
	if p.autoUpdate {
		go p.updateLoop()
	}
	return nil
}

func (p *program) runUI() {
	// Use unified 'up' command with --no-browser for service mode
	args := []string{"up", "--no-browser", "-p", p.uiPort}

	p.uiCmd = exec.Command(p.xplatBin, args...)
	p.uiCmd.Dir = p.workDir
	p.uiCmd.Stdout = os.Stdout
	p.uiCmd.Stderr = os.Stderr
	p.uiCmd.Env = config.FullEnv(p.workDir)

	log.Printf("Starting xplat UI on port %s...", p.uiPort)
	if err := p.uiCmd.Run(); err != nil {
		log.Printf("xplat UI exited: %v", err)
	}
}

func (p *program) runMCP() {
	// Build MCP command args for HTTP mode
	args := []string{"mcp", "serve", "--http", ":" + p.mcpPort}

	p.mcpCmd = exec.Command(p.xplatBin, args...)
	p.mcpCmd.Dir = p.workDir
	p.mcpCmd.Stdout = os.Stdout
	p.mcpCmd.Stderr = os.Stderr
	p.mcpCmd.Env = config.FullEnv(p.workDir)

	log.Printf("Starting MCP HTTP server on port %s...", p.mcpPort)
	if err := p.mcpCmd.Run(); err != nil {
		log.Printf("MCP server exited: %v", err)
	}
}

func (p *program) runSync() {
	// Build sync-gh poll command args
	// If repos not specified, poll command will auto-discover from Taskfile.yml
	args := []string{"sync-gh", "poll", "--interval=" + p.syncInterval, "--invalidate"}
	if p.syncRepos != "" {
		args = append(args, "--repos="+p.syncRepos)
	}

	p.syncCmd = exec.Command(p.xplatBin, args...)
	p.syncCmd.Dir = p.workDir
	p.syncCmd.Stdout = os.Stdout
	p.syncCmd.Stderr = os.Stderr
	p.syncCmd.Env = config.FullEnv(p.workDir)

	if p.syncRepos != "" {
		log.Printf("Starting GitHub sync poller (repos: %s, interval: %s)...", p.syncRepos, p.syncInterval)
	} else {
		log.Printf("Starting GitHub sync poller (auto-discover from Taskfile.yml, interval: %s)...", p.syncInterval)
	}
	if err := p.syncCmd.Run(); err != nil {
		log.Printf("GitHub sync poller exited: %v", err)
	}
}

func (p *program) run() {
	// Build command args: start with "process"
	args := []string{"process"}

	// Load all enabled project configs from registry
	reg, err := projects.Load()
	if err != nil {
		log.Printf("Warning: failed to load project registry: %v", err)
	} else {
		configFiles := reg.EnabledConfigFiles()
		if len(configFiles) > 0 {
			log.Printf("Loading %d project config(s) from registry", len(configFiles))
			for _, cfg := range configFiles {
				args = append(args, "-f", cfg)
			}
		}
	}

	// Add headless mode flags (no TUI, no server, suitable for service)
	args = append(args, "-t=false", "--no-server")

	// Run process-compose with all configs
	p.cmd = exec.Command(p.xplatBin, args...)
	p.cmd.Dir = p.workDir
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	// Use xplat paths environment: PLAT_* vars + PLAT_BIN/XPLAT_BIN in PATH
	p.cmd.Env = config.FullEnv(p.workDir)

	log.Printf("Running: %s %s", p.xplatBin, strings.Join(args, " "))

	if err := p.cmd.Run(); err != nil {
		log.Printf("xplat process exited: %v", err)
	}
}

func (p *program) updateLoop() {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	// Check immediately on startup
	p.checkAndUpdate()

	for {
		select {
		case <-ticker.C:
			p.checkAndUpdate()
		case <-p.stopChan:
			return
		}
	}
}

func (p *program) checkAndUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	latest, err := updater.GetLatestVersion(ctx)
	if err != nil {
		log.Printf("Auto-update: failed to check for updates: %v", err)
		return
	}

	// Skip if already up to date or running dev version
	if !updater.NeedsUpdate(p.version, latest) {
		return
	}

	log.Printf("Auto-update: new version available: %s -> %s", p.version, latest)

	// Get the download URL
	release, err := updater.GetLatestRelease(ctx)
	if err != nil {
		log.Printf("Auto-update: failed to get release info: %v", err)
		return
	}

	downloadURL, err := updater.FindAssetURL(release)
	if err != nil {
		log.Printf("Auto-update: %v", err)
		return
	}

	// Fetch checksums for verification
	expectedChecksum := updater.GetExpectedChecksum(ctx, release)
	if expectedChecksum == "" {
		log.Printf("Auto-update: warning: checksum not available, skipping verification")
	}

	// Download and replace
	dlCtx, dlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer dlCancel()

	if err := updater.DownloadAndReplace(dlCtx, downloadURL, p.xplatBin, expectedChecksum); err != nil {
		log.Printf("Auto-update: failed to update: %v", err)
		return
	}

	log.Printf("Auto-update: updated to %s, restarting...", latest)

	// Restart by exiting - the service manager will restart us
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	os.Exit(0)
}

func (p *program) Stop(s service.Service) error {
	log.Printf("Stopping %s service...", s.String())
	if p.stopChan != nil {
		close(p.stopChan)
	}
	if p.cmd != nil && p.cmd.Process != nil {
		// Send SIGTERM/SIGINT to gracefully stop
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	if p.uiCmd != nil && p.uiCmd.Process != nil {
		_ = p.uiCmd.Process.Signal(os.Interrupt)
	}
	if p.mcpCmd != nil && p.mcpCmd.Process != nil {
		_ = p.mcpCmd.Process.Signal(os.Interrupt)
	}
	if p.syncCmd != nil && p.syncCmd.Process != nil {
		_ = p.syncCmd.Process.Signal(os.Interrupt)
	}
	return nil
}

// Manager manages service lifecycle operations.
type Manager struct {
	svc    service.Service
	config Config
}

// NewManager creates a new service manager.
func NewManager(cfg Config) (*Manager, error) {
	// Find xplat binary path
	xplatBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	svcConfig := &service.Config{
		Name:             cfg.Name,
		DisplayName:      cfg.DisplayName,
		Description:      cfg.Description,
		WorkingDirectory: cfg.WorkDir,
		Arguments:        []string{"service", "run"},
	}

	if cfg.UserService {
		svcConfig.Option = service.KeyValue{
			"UserService": true,
		}
	}

	// Default UI port if not set
	uiPort := cfg.UIPort
	if uiPort == "" {
		uiPort = "3000"
	}

	// Default MCP port if not set
	mcpPort := cfg.MCPPort
	if mcpPort == "" {
		mcpPort = "8765"
	}

	// Default sync interval if not set
	syncInterval := cfg.SyncInterval
	if syncInterval == "" {
		syncInterval = config.DefaultSyncInterval
	}

	prg := &program{
		workDir:      cfg.WorkDir,
		xplatBin:     xplatBin,
		autoUpdate:   cfg.AutoUpdate,
		version:      cfg.Version,
		withUI:       cfg.WithUI,
		uiPort:       uiPort,
		withMCP:      cfg.WithMCP,
		mcpPort:      mcpPort,
		withSync:     cfg.WithSync,
		syncRepos:    cfg.SyncRepos,
		syncInterval: syncInterval,
	}

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	return &Manager{
		svc:    svc,
		config: cfg,
	}, nil
}

// Install installs the service.
func (m *Manager) Install() error {
	err := m.svc.Install()
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("service %s already installed", m.config.Name)
		}
		return fmt.Errorf("failed to install: %w", err)
	}
	return nil
}

// Uninstall removes the service.
func (m *Manager) Uninstall() error {
	err := m.svc.Uninstall()
	if err != nil {
		if strings.Contains(err.Error(), "not installed") {
			return fmt.Errorf("service %s not installed", m.config.Name)
		}
		return fmt.Errorf("failed to uninstall: %w", err)
	}
	return nil
}

// Start starts the service.
func (m *Manager) Start() error {
	status, _ := m.svc.Status()
	if status == service.StatusRunning {
		return fmt.Errorf("service %s already running", m.config.Name)
	}

	err := m.svc.Start()
	if err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	return nil
}

// Stop stops the service.
func (m *Manager) Stop() error {
	status, _ := m.svc.Status()
	if status == service.StatusStopped {
		return fmt.Errorf("service %s already stopped", m.config.Name)
	}

	err := m.svc.Stop()
	if err != nil {
		return fmt.Errorf("failed to stop: %w", err)
	}
	return nil
}

// Restart restarts the service.
func (m *Manager) Restart() error {
	err := m.svc.Restart()
	if err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}
	return nil
}

// Status returns the service status.
func (m *Manager) Status() (string, error) {
	status, err := m.svc.Status()
	if err != nil {
		return "unknown", err
	}

	switch status {
	case service.StatusRunning:
		return "running", nil
	case service.StatusStopped:
		return "stopped", nil
	default:
		return "unknown", nil
	}
}

// Run runs the service (called when service starts).
func (m *Manager) Run() error {
	return m.svc.Run()
}

// Platform returns the service platform (e.g., "darwin-launchd", "linux-systemd").
func (m *Manager) Platform() string {
	return service.Platform()
}
