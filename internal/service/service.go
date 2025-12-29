// Package service provides cross-platform system service management using kardianos/service.
package service

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kardianos/service"
)

// Config holds service configuration.
type Config struct {
	Name        string // Service name (e.g., "xplat" or "xplat-myproject")
	DisplayName string // Human-readable name
	Description string // Service description
	WorkDir     string // Working directory for the service
	UserService bool   // Install as user service (not root)
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
	}
}

// program implements the service.Interface.
type program struct {
	cmd     *exec.Cmd
	workDir string
	xplatBin string
}

func (p *program) Start(s service.Service) error {
	log.Printf("Starting %s service...", s.String())
	go p.run()
	return nil
}

func (p *program) run() {
	// Run xplat dev (process-compose) in foreground mode
	p.cmd = exec.Command(p.xplatBin, "dev")
	p.cmd.Dir = p.workDir
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	// Ensure PATH includes common binary locations
	p.cmd.Env = append(os.Environ(),
		"PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:"+os.Getenv("PATH"),
	)

	if err := p.cmd.Run(); err != nil {
		log.Printf("xplat dev exited: %v", err)
	}
}

func (p *program) Stop(s service.Service) error {
	log.Printf("Stopping %s service...", s.String())
	if p.cmd != nil && p.cmd.Process != nil {
		// Send SIGTERM/SIGINT to gracefully stop
		p.cmd.Process.Signal(os.Interrupt)
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

	prg := &program{
		workDir:  cfg.WorkDir,
		xplatBin: xplatBin,
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
