// Package web provides a unified Via web application for xplat.
//
// This consolidates all web UI components (Tasks, Processes, Setup) into
// a single Via instance with shared navigation and configuration.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-via/via"
	"github.com/go-via/via-plugin-picocss/picocss"
	"github.com/go-via/via/h"

	"github.com/joeblew999/xplat/internal/config"
)

// AppConfig holds the unified web application configuration.
type AppConfig struct {
	Port               string // Port to listen on (default 8760)
	Taskfile           string // Path to Taskfile.yml
	WorkDir            string // Working directory
	OpenBrowser        bool   // Open browser on start
	ProcessComposePort int    // Process-compose API port
	EnableSetup        bool   // Enable setup wizard routes
	EnableTasks        bool   // Enable task UI routes
	EnableProcesses    bool   // Enable process view routes
	MockMode           bool   // Mock mode for setup wizard
}

// DefaultAppConfig returns sensible defaults with all features enabled.
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Port:               config.DefaultUIPort,
		Taskfile:           config.DefaultTaskfile,
		WorkDir:            "",
		OpenBrowser:        config.DefaultOpenBrowser,
		ProcessComposePort: config.DefaultProcessComposePort,
		EnableSetup:        true,
		EnableTasks:        true,
		EnableProcesses:    true,
		MockMode:           false,
	}
}

// App represents the unified xplat web application.
type App struct {
	config   AppConfig
	via      *via.V
	tasks    []TaskInfo
	pcClient *ProcessComposeClient
}

// NewApp creates a new unified web application.
func NewApp(cfg AppConfig) (*App, error) {
	if cfg.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cfg.WorkDir = wd
	}

	app := &App{
		config: cfg,
	}

	// Parse taskfile if tasks are enabled
	if cfg.EnableTasks {
		tasks, err := listTasksFromFile(cfg.Taskfile, cfg.WorkDir)
		if err != nil {
			log.Printf("Warning: Failed to parse taskfile: %v", err)
			tasks = []TaskInfo{}
		}
		app.tasks = tasks
	}

	// Create process-compose client if processes are enabled
	if cfg.EnableProcesses {
		app.pcClient = NewProcessComposeClient(cfg.ProcessComposePort)
	}

	return app, nil
}

// Start starts the unified web application.
func (app *App) Start(ctx context.Context) error {
	url := fmt.Sprintf("http://localhost:%s", app.config.Port)
	log.Printf("xplat UI listening on %s\n", url)

	if app.config.OpenBrowser {
		go openBrowser(url)
	}

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			log.Println("\nShutting down...")
			os.Exit(0)
		case <-ctx.Done():
			os.Exit(0)
		}
	}()

	// Create and configure Via instance
	app.via = via.New()
	app.via.Config(via.Options{
		DocumentTitle: "xplat",
		Plugins:       []via.Plugin{picocss.Default},
		DevMode:       os.Getenv("VIA_DEV_MODE") != "false",
		LogLvl:        via.LogLevelWarn,
		ServerAddress: ":" + app.config.Port,
	})

	// Register routes based on enabled features
	app.registerRoutes()

	// Start the server
	app.via.Start()
	return nil
}

// registerRoutes registers all enabled routes.
func (app *App) registerRoutes() {
	// Always register the unified index page
	app.via.Page("/", func(c *via.Context) {
		app.unifiedIndexPage(c)
	})

	// Task routes
	if app.config.EnableTasks {
		app.via.Page("/tasks", func(c *via.Context) {
			viaTaskListPage(c, app.tasks, ViaConfig{
				Port:               app.config.Port,
				Taskfile:           app.config.Taskfile,
				WorkDir:            app.config.WorkDir,
				ProcessComposePort: app.config.ProcessComposePort,
			})
		})

		// Register each task as a separate route
		for _, task := range app.tasks {
			taskName := task.Name
			taskDesc := task.Description
			app.via.Page("/tasks/"+taskName, func(c *via.Context) {
				viaTaskExecutionPage(c, taskName, taskDesc, app.tasks, ViaConfig{
					Port:               app.config.Port,
					Taskfile:           app.config.Taskfile,
					WorkDir:            app.config.WorkDir,
					ProcessComposePort: app.config.ProcessComposePort,
				})
			})
		}
	}

	// Process routes
	if app.config.EnableProcesses {
		app.via.Page("/processes", func(c *via.Context) {
			viaProcessListPage(c, app.pcClient, ViaConfig{
				Port:               app.config.Port,
				Taskfile:           app.config.Taskfile,
				WorkDir:            app.config.WorkDir,
				ProcessComposePort: app.config.ProcessComposePort,
			})
		})

		// API endpoint for process logs
		app.via.HandleFunc("GET /api/process/logs/{name}", func(w http.ResponseWriter, r *http.Request) {
			processName := r.PathValue("name")
			if processName == "" {
				http.Error(w, "process name required", http.StatusBadRequest)
				return
			}

			logs, err := app.pcClient.GetProcessLogs(processName, 500)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"logs": logs})
		})
	}

	// Setup wizard routes
	if app.config.EnableSetup {
		app.registerSetupRoutes()
	}
}

// registerSetupRoutes registers the setup wizard routes.
func (app *App) registerSetupRoutes() {
	// Setup routes will be under /setup/* prefix
	// This will be integrated with the existing env/web routes
	app.via.Page("/setup", func(c *via.Context) {
		app.setupIndexPage(c)
	})
}

// ActiveTab represents the currently active navigation tab.
type ActiveTab string

const (
	TabHome      ActiveTab = "home"
	TabTasks     ActiveTab = "tasks"
	TabProcesses ActiveTab = "processes"
	TabSetup     ActiveTab = "setup"
)

// unifiedIndexPage renders the main landing page with all sections.
func (app *App) unifiedIndexPage(c *via.Context) {
	c.View(func() h.H {
		return h.Div(
			// Unified header
			app.renderNav(TabHome),

			// Main content - dashboard style
			h.Main(
				h.Class("container"),
				h.H1(h.Text("xplat Dashboard")),
				h.P(
					h.Style("color: var(--pico-muted-color);"),
					h.Text(app.config.WorkDir),
				),

				// Feature cards
				h.Div(
					h.Style("display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 1rem; margin-top: 2rem;"),

					// Tasks card
					h.If(app.config.EnableTasks,
						h.Article(
							h.H3(h.Text("Tasks")),
							h.P(h.Text(fmt.Sprintf("%d tasks available from %s", len(app.tasks), app.config.Taskfile))),
							h.A(
								h.Href("/tasks"),
								h.Attr("role", "button"),
								h.Text("View Tasks"),
							),
						),
					),

					// Processes card
					h.If(app.config.EnableProcesses,
						h.Article(
							h.H3(h.Text("Processes")),
							h.P(h.Text("Monitor and control running processes")),
							h.A(
								h.Href("/processes"),
								h.Attr("role", "button"),
								h.Text("View Processes"),
							),
						),
					),

					// Setup card
					h.If(app.config.EnableSetup,
						h.Article(
							h.H3(h.Text("Setup")),
							h.P(h.Text("Configure environment and external services")),
							h.A(
								h.Href("/setup"),
								h.Attr("role", "button"),
								h.Text("Open Setup Wizard"),
							),
						),
					),
				),
			),
		)
	})
}

// setupIndexPage renders the setup wizard landing page.
func (app *App) setupIndexPage(c *via.Context) {
	c.View(func() h.H {
		return h.Div(
			app.renderNav(TabSetup),
			h.Main(
				h.Class("container"),
				h.H1(h.Text("Environment Setup")),
				h.P(h.Text("Configure your environment and external services.")),
				h.Article(
					h.H3(h.Text("Coming Soon")),
					h.P(h.Text("The setup wizard will be integrated here.")),
				),
			),
		)
	})
}

// renderNav renders the unified navigation header.
// This is a wrapper around the shared RenderNav function.
func (app *App) renderNav(activeTab ActiveTab) h.H {
	return RenderNav(string(activeTab), app.config.WorkDir)
}
