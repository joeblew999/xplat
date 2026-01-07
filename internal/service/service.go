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

	"github.com/joeblew999/xplat/internal/paths"
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
	uiCmd      *exec.Cmd // Task UI process (if WithUI is enabled)
	workDir    string
	xplatBin   string
	autoUpdate bool
	version    string
	withUI     bool
	uiPort     string
	stopChan   chan struct{}
}

func (p *program) Start(s service.Service) error {
	log.Printf("Starting %s service...", s.String())
	p.stopChan = make(chan struct{})
	go p.run()
	if p.withUI {
		go p.runUI()
	}
	if p.autoUpdate {
		go p.updateLoop()
	}
	return nil
}

func (p *program) runUI() {
	// Build UI command args (Via mode is now the only mode)
	args := []string{"ui", "--no-browser", "-p", p.uiPort}

	p.uiCmd = exec.Command(p.xplatBin, args...)
	p.uiCmd.Dir = p.workDir
	p.uiCmd.Stdout = os.Stdout
	p.uiCmd.Stderr = os.Stderr
	p.uiCmd.Env = paths.FullEnv(p.workDir)

	log.Printf("Starting Task UI on port %s...", p.uiPort)
	if err := p.uiCmd.Run(); err != nil {
		log.Printf("Task UI exited: %v", err)
	}
}

func (p *program) run() {
	// Run process-compose in headless mode (no TUI, no server, suitable for service)
	p.cmd = exec.Command(p.xplatBin, "process", "-t=false", "--no-server")
	p.cmd.Dir = p.workDir
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	// Use xplat paths environment: PLAT_* vars + PLAT_BIN/XPLAT_BIN in PATH
	p.cmd.Env = paths.FullEnv(p.workDir)

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

	// Download and replace
	dlCtx, dlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer dlCancel()

	if err := updater.DownloadAndReplace(dlCtx, downloadURL, p.xplatBin); err != nil {
		log.Printf("Auto-update: failed to update: %v", err)
		return
	}

	log.Printf("Auto-update: updated to %s, restarting...", latest)

	// Restart by exiting - the service manager will restart us
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Signal(os.Interrupt)
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
		p.cmd.Process.Signal(os.Interrupt)
	}
	if p.uiCmd != nil && p.uiCmd.Process != nil {
		p.uiCmd.Process.Signal(os.Interrupt)
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

	prg := &program{
		workDir:    cfg.WorkDir,
		xplatBin:   xplatBin,
		autoUpdate: cfg.AutoUpdate,
		version:    cfg.Version,
		withUI:     cfg.WithUI,
		uiPort:     uiPort,
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
