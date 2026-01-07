// Package taskui provides a web-based UI for running Taskfile tasks.
//
// Inspired by github.com/titpetric/task-ui (GPL-3.0 license).
// Original project: https://github.com/titpetric/task-ui
//
// Key differences from the original:
//   - Uses xplat's embedded Task runner instead of shelling out to bash
//   - Simplified templates using CDN-hosted xterm.js and Bootstrap
//   - Cross-platform support (Unix PTY, Windows pipes fallback)
//   - Integrated into xplat CLI as `xplat ui` command
//
// Static assets sourced from CDNs:
//   - xterm.js v5.3.0: https://cdn.jsdelivr.net/npm/xterm@5.3.0/
//   - Bootstrap v5.3.2: https://cdn.jsdelivr.net/npm/bootstrap@5.3.2/
package taskui

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed templates/*.html static/*
var embeddedFiles embed.FS

// Config holds the server configuration.
type Config struct {
	ListenAddr string // Address to listen on (default ":3000")
	Taskfile   string // Path to Taskfile.yml (default "Taskfile.yml")
	WorkDir    string // Working directory for task execution
	OpenBrowser bool  // Open browser on start
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		ListenAddr: ":3000",
		Taskfile:   "Taskfile.yml",
		WorkDir:    "",
		OpenBrowser: true,
	}
}

// Server is the task-ui web server.
type Server struct {
	config    Config
	templates *template.Template
	mu        sync.Mutex
	running   string // Currently running task (empty if none)
}

// New creates a new task-ui server.
func New(cfg Config) (*Server, error) {
	if cfg.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cfg.WorkDir = wd
	}

	// Parse templates
	tmpl, err := template.ParseFS(embeddedFiles, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		config:    cfg,
		templates: tmpl,
	}, nil
}

// Start starts the web server.
func (s *Server) Start(ctx context.Context) error {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Routes
	r.Get("/", s.handleIndex)
	r.Get("/task/{name}", s.handleTask)
	r.Get("/api/tasks", s.handleAPITasks)
	r.Handle("/ws/{name}", s.handleWebSocket())

	// Serve static files
	staticFS, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to get static files: %w", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Parse listen address
	_, port, err := net.SplitHostPort(s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%s", port)
	fmt.Printf("Task UI listening on %s\n", url)

	if s.config.OpenBrowser {
		go openBrowser(url)
	}

	server := &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: r,
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

// CanRun returns true if no task is currently running.
func (s *Server) CanRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running == ""
}

// SetRunning marks a task as running.
func (s *Server) SetRunning(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != "" {
		return false
	}
	s.running = name
	return true
}

// ClearRunning clears the running task.
func (s *Server) ClearRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = ""
}
