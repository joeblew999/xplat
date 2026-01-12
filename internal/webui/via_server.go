// Package taskui provides a web-based UI for running Taskfile tasks.
//
// This file implements the Via/Datastar version using SSE for bidirectional
// communication instead of WebSockets.
//
// Inspired by github.com/titpetric/task-ui (GPL-3.0 license).
// Original project: https://github.com/titpetric/task-ui
package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-via/via"
	"github.com/go-via/via-plugin-picocss/picocss"
	"github.com/go-via/via/h"

	"github.com/joeblew999/xplat/internal/config"
)

// TaskInfo holds information about a task.
type TaskInfo struct {
	Name        string
	Description string
	Summary     string
}

// ViaConfig holds the Via server configuration.
type ViaConfig struct {
	Port              string // Port to listen on (default "3000")
	Taskfile          string // Path to Taskfile.yml (default "Taskfile.yml")
	WorkDir           string // Working directory for task execution
	OpenBrowser       bool   // Open browser on start
	ProcessComposePort int    // Port for process-compose API (default 8080)
}

// DefaultViaConfig returns sensible defaults.
func DefaultViaConfig() ViaConfig {
	return ViaConfig{
		Port:               config.DefaultUIPort,
		Taskfile:           config.DefaultTaskfile,
		WorkDir:            "",
		OpenBrowser:        config.DefaultOpenBrowser,
		ProcessComposePort: config.DefaultProcessComposePort,
	}
}

// StartVia starts the Via-based task UI server.
func StartVia(ctx context.Context, cfg ViaConfig) error {
	if cfg.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg.WorkDir = wd
	}

	url := fmt.Sprintf("http://localhost:%s", cfg.Port)
	log.Printf("Task UI (Via) listening on %s\n", url)

	if cfg.OpenBrowser {
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

	v := via.New()
	v.Config(via.Options{
		DocumentTitle: "xplat Task UI",
		Plugins:       []via.Plugin{picocss.Default},
		DevMode:       os.Getenv("VIA_DEV_MODE") != "false",
		LogLvl:        via.LogLevelWarn,
		ServerAddress: ":" + cfg.Port,
	})

	// Parse taskfile once at startup
	tasks, err := listTasksFromFile(cfg.Taskfile, cfg.WorkDir)
	if err != nil {
		log.Printf("Warning: Failed to parse taskfile: %v", err)
		tasks = []TaskInfo{}
	}

	// Create process-compose client
	pcClient := NewProcessComposeClient(cfg.ProcessComposePort)

	// API endpoint to proxy process logs (avoids CORS issues) - register before pages
	v.HandleFunc("GET /api/process/logs/{name}", func(w http.ResponseWriter, r *http.Request) {
		processName := r.PathValue("name")
		if processName == "" {
			http.Error(w, "process name required", http.StatusBadRequest)
			return
		}

		logs, err := pcClient.GetProcessLogs(processName, 500)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"logs": logs})
	})

	// Index page - task list
	v.Page("/", func(c *via.Context) {
		viaTaskListPage(c, tasks, cfg)
	})

	// Processes page - process-compose status
	v.Page("/processes", func(c *via.Context) {
		viaProcessListPage(c, pcClient, cfg)
	})

	// Task execution pages - register one page per task
	// Via doesn't support path parameters, so we register each task as a separate route
	for _, task := range tasks {
		taskName := task.Name // capture for closure
		taskDesc := task.Description
		v.Page("/task/"+taskName, func(c *via.Context) {
			viaTaskExecutionPage(c, taskName, taskDesc, tasks, cfg)
		})
	}

	v.Start()
	return nil
}

// listTasksFromFile parses the taskfile and returns task info
func listTasksFromFile(filename, workDir string) ([]TaskInfo, error) {
	tf, err := parseTaskfile(filename, workDir)
	if err != nil {
		return nil, err
	}

	var tasks []TaskInfo
	for name, task := range tf.Tasks {
		// Skip internal tasks
		if len(name) > 0 && name[0] == '_' {
			continue
		}
		if task.Internal {
			continue
		}
		tasks = append(tasks, TaskInfo{
			Name:        name,
			Description: task.Desc,
			Summary:     task.Summary,
		})
	}

	// Sort by name (simple bubble sort for small lists)
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[i].Name > tasks[j].Name {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}

	return tasks, nil
}

// viaTaskListPage renders the task list using Via
func viaTaskListPage(c *via.Context, tasks []TaskInfo, cfg ViaConfig) {
	c.View(func() h.H {
		// Build task links
		var taskLinks []h.H
		for _, t := range tasks {
			taskLinks = append(taskLinks,
				h.A(
					h.Href("/task/"+t.Name),
					h.Style("display: flex; justify-content: space-between; align-items: center; padding: 0.75rem 1rem; border-bottom: 1px solid var(--pico-muted-border-color); text-decoration: none;"),
					h.Div(
						h.Strong(h.Text(t.Name)),
						h.If(t.Description != "",
							h.Div(
								h.Small(
									h.Style("color: var(--pico-muted-color);"),
									h.Text(t.Description),
								),
							),
						),
					),
					h.Span(
						h.Style("background-color: var(--pico-primary); color: white; padding: 0.25rem 0.5rem; border-radius: 0.25rem;"),
						h.Text("▶"),
					),
				),
			)
		}

		return h.Div(
			// Header with tabs
			viaNavHeader("tasks", cfg),

			// Main content
			h.Main(
				h.Class("container"),
				h.Article(
					h.H3(h.Text("Available Tasks")),
					h.If(len(taskLinks) > 0,
						h.Div(taskLinks...),
					),
					h.If(len(taskLinks) == 0,
						h.P(
							h.Style("text-align: center; color: var(--pico-muted-color);"),
							h.Text("No tasks found in "+cfg.Taskfile),
						),
					),
				),
			),
		)
	})
}

// groupTasksByNamespace organizes tasks by their prefix (namespace)
func groupTasksByNamespace(tasks []TaskInfo) map[string][]TaskInfo {
	groups := make(map[string][]TaskInfo)
	for _, t := range tasks {
		// Extract namespace from task name (e.g., "check:deps" -> "check")
		namespace := ""
		if idx := strings.Index(t.Name, ":"); idx > 0 {
			namespace = t.Name[:idx]
		}
		groups[namespace] = append(groups[namespace], t)
	}
	return groups
}

// getNamespaceOrder returns namespaces sorted, with "" (root) first
func getNamespaceOrder(groups map[string][]TaskInfo) []string {
	var namespaces []string
	for ns := range groups {
		namespaces = append(namespaces, ns)
	}
	// Sort namespaces alphabetically
	for i := 0; i < len(namespaces)-1; i++ {
		for j := i + 1; j < len(namespaces); j++ {
			if namespaces[i] > namespaces[j] {
				namespaces[i], namespaces[j] = namespaces[j], namespaces[i]
			}
		}
	}
	// Move "" (root tasks) to front
	for i, ns := range namespaces {
		if ns == "" {
			namespaces = append([]string{""}, append(namespaces[:i], namespaces[i+1:]...)...)
			break
		}
	}
	return namespaces
}

// viaTaskExecutionPage renders the task execution page with terminal output
func viaTaskExecutionPage(c *via.Context, taskName, taskDesc string, tasks []TaskInfo, cfg ViaConfig) {
	// Signals for state management
	output := c.Signal("")
	status := c.Signal("ready") // ready, running, finished, error
	running := c.Signal(false)

	// Run task action
	runAction := c.Action(func() {
		if running.String() == "true" {
			return
		}

		running.SetValue(true)
		status.SetValue("running")
		output.SetValue("")
		c.Sync()

		// Run the task and stream output
		go func() {
			err := runTaskWithCallback(taskName, cfg.WorkDir, func(line string) {
				// Append output line
				current := output.String()
				if current != "" {
					current += "\n"
				}
				output.SetValue(current + line)
				c.Sync()
			})

			running.SetValue(false)
			if err != nil {
				status.SetValue("error")
			} else {
				status.SetValue("finished")
			}
			c.Sync()
		}()
	})

	c.View(func() h.H {
		statusText := "Ready to run"
		switch status.String() {
		case "running":
			statusText = "Running..."
		case "finished":
			statusText = "Finished"
		case "error":
			statusText = "Error"
		}

		// Get current task's namespace
		currentNamespace := ""
		if idx := strings.Index(taskName, ":"); idx > 0 {
			currentNamespace = taskName[:idx]
		}

		// Group tasks by namespace for organized sidebar
		groups := groupTasksByNamespace(tasks)
		namespaces := getNamespaceOrder(groups)

		// Build sidebar with collapsible groups
		var sidebarLinks []h.H
		for _, ns := range namespaces {
			groupTasks := groups[ns]
			isCurrentGroup := ns == currentNamespace

			// Group header (if namespace exists)
			if ns != "" {
				headerStyle := "display: block; padding: 0.5rem 0.5rem; font-weight: bold; font-size: 0.85rem; color: var(--pico-muted-color); border-bottom: 1px solid var(--pico-muted-border-color); margin-top: 0.5rem;"
				if isCurrentGroup {
					headerStyle = "display: block; padding: 0.5rem 0.5rem; font-weight: bold; font-size: 0.85rem; color: var(--pico-primary); border-bottom: 1px solid var(--pico-primary); margin-top: 0.5rem;"
				}
				sidebarLinks = append(sidebarLinks, h.Div(
					h.Style(headerStyle),
					h.Text(ns+":"),
				))
			}

			// Task links in this group
			for _, t := range groupTasks {
				isActive := t.Name == taskName
				// Get display name (without namespace prefix if in a group)
				displayName := t.Name
				if ns != "" && strings.HasPrefix(t.Name, ns+":") {
					displayName = strings.TrimPrefix(t.Name, ns+":")
				}

				style := "display: block; padding: 0.35rem 0.5rem; text-decoration: none; border-radius: 0.25rem; margin-bottom: 0.15rem; font-size: 0.9rem;"
				if ns != "" {
					style += " padding-left: 1rem;" // Indent namespaced tasks
				}
				if isActive {
					style += " background-color: var(--pico-primary); color: white;"
				}
				link := h.A(
					h.Href("/task/"+t.Name),
					h.Style(style),
					h.Text(displayName),
				)
				if isActive {
					link = h.A(
						h.Href("/task/"+t.Name),
						h.Style(style),
						h.ID("active-task"),
						h.Text(displayName),
					)
				}
				sidebarLinks = append(sidebarLinks, link)
			}
		}

		return h.Div(
			// Header with tabs
			viaNavHeader("tasks", cfg),

			// Main content
			h.Main(
				h.Class("container"),
				h.Div(
					h.Style("display: grid; grid-template-columns: 200px 1fr; gap: 1rem;"),

					// Sidebar - task list
					h.Aside(
						h.Style("position: sticky; top: 1rem; align-self: start;"),
						h.Article(
							h.H4(h.Text("Tasks")),
							h.Div(
								h.Style("max-height: calc(100vh - 150px); overflow-y: auto;"),
								h.Div(sidebarLinks...),
							),
						),
					),

					// Main task area
					h.Div(
						h.Article(
							// Header with task name and buttons
							h.Div(
								h.Style("display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;"),
								h.Div(
									h.H3(
										h.Style("margin: 0;"),
										h.Code(h.Text("task "+taskName)),
									),
									h.If(taskDesc != "",
										h.Small(
											h.Style("color: var(--pico-muted-color);"),
											h.Text(taskDesc),
										),
									),
								),
								h.Div(
									h.Style("display: flex; gap: 0.5rem;"),
									h.Button(
										h.Text("▶ Run"),
										h.If(running.String() == "true", h.Attr("aria-busy", "true")),
										h.If(running.String() == "true", h.Attr("disabled", "disabled")),
										runAction.OnClick(),
									),
									h.A(
										h.Href("/"),
										h.Class("secondary"),
										h.Attr("role", "button"),
										h.Text("← Back"),
									),
								),
							),

							// Terminal output area
							h.Div(
								h.Style("background-color: #1e1e1e; color: #d4d4d4; padding: 1rem; border-radius: 0.5rem; min-height: 300px; font-family: 'Menlo', 'Monaco', 'Courier New', monospace; font-size: 14px; white-space: pre-wrap; overflow-y: auto; max-height: 500px;"),
								h.If(output.String() == "" && status.String() == "ready",
									h.Span(
										h.Style("color: #6c757d;"),
										h.Text("Click \"Run\" to execute: task "+taskName),
									),
								),
								h.If(output.String() != "",
									h.Text(output.String()),
								),
							),

							// Status footer
							h.Div(
								h.Style("margin-top: 0.5rem; color: var(--pico-muted-color);"),
								h.Small(h.Text(statusText)),
							),
						),
					),
				),
			),
		)
	})
}

// runTaskWithCallback runs a task and calls the callback for each line of output
func runTaskWithCallback(taskName, workDir string, callback func(string)) error {
	xplatBin, err := os.Executable()
	if err != nil {
		xplatBin = "xplat"
	}

	cmd := exec.Command(xplatBin, "task", taskName)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1")

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Read output in goroutines
	done := make(chan struct{})
	go func() {
		readLines(stdout, callback)
		done <- struct{}{}
	}()
	go func() {
		readLines(stderr, callback)
		done <- struct{}{}
	}()

	// Wait for both readers
	<-done
	<-done

	return cmd.Wait()
}

func readLines(r io.Reader, callback func(string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		callback(scanner.Text())
	}
}

// viaNavHeader creates the navigation header with tabs.
func viaNavHeader(activeTab string, cfg ViaConfig) h.H {
	taskTabStyle := "color: white; text-decoration: none; padding: 0.5rem 1rem; border-radius: 0.25rem 0.25rem 0 0;"
	processTabStyle := taskTabStyle

	if activeTab == "tasks" {
		taskTabStyle += " background-color: #495057;"
	}
	if activeTab == "processes" {
		processTabStyle += " background-color: #495057;"
	}

	return h.Nav(
		h.Style("background-color: #343a40; padding: 1rem; margin-bottom: 1rem;"),
		h.Div(
			h.Style("display: flex; justify-content: space-between; align-items: center; max-width: 1200px; margin: 0 auto;"),
			h.Div(
				h.Style("display: flex; align-items: center; gap: 1rem;"),
				h.A(
					h.Href("/"),
					h.Style("color: white; text-decoration: none; font-size: 1.25rem;"),
					h.Strong(h.Text("xplat ")),
					h.Text("UI"),
				),
				h.Div(
					h.Style("display: flex; gap: 0.25rem; margin-left: 1rem;"),
					h.A(
						h.Href("/"),
						h.Style(taskTabStyle),
						h.Text("Tasks"),
					),
					h.A(
						h.Href("/processes"),
						h.Style(processTabStyle),
						h.Text("Processes"),
					),
				),
			),
			h.Span(
				h.Style("color: #6c757d;"),
				h.Text(cfg.WorkDir),
			),
		),
	)
}

// viaProcessListPage renders the process-compose status page.
func viaProcessListPage(c *via.Context, client *ProcessComposeClient, cfg ViaConfig) {
	// Signals for state management
	refreshing := c.Signal(false)
	processData := c.Signal("[]")
	errorMsg := c.Signal("")
	lastRefresh := c.Signal("")
	autoRefresh := c.Signal(true) // Auto-refresh enabled by default

	// Fetch processes function
	fetchProcesses := func() {
		processes, err := client.ListProcesses()
		if err != nil {
			errorMsg.SetValue(err.Error())
			processData.SetValue("[]")
		} else {
			errorMsg.SetValue("")
			data, _ := json.Marshal(processes)
			processData.SetValue(string(data))
		}
		lastRefresh.SetValue(fmt.Sprintf("Last updated: %s", time.Now().Format("15:04:05")))
		c.Sync()
	}

	// Refresh action
	refreshAction := c.Action(func() {
		if refreshing.String() == "true" {
			return
		}
		refreshing.SetValue(true)
		c.Sync()

		go func() {
			fetchProcesses()
			refreshing.SetValue(false)
			c.Sync()
		}()
	})

	// Toggle auto-refresh action
	toggleAutoRefresh := c.Action(func() {
		if autoRefresh.String() == "true" {
			autoRefresh.SetValue(false)
		} else {
			autoRefresh.SetValue(true)
		}
		c.Sync()
	})

	// Initial fetch
	go fetchProcesses()

	// Auto-refresh loop (every 3 seconds when enabled)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if autoRefresh.String() == "true" {
				fetchProcesses()
			}
		}
	}()

	c.View(func() h.H {
		// Parse process data
		var processes []ProcessInfo
		_ = json.Unmarshal([]byte(processData.String()), &processes)

		// Check if process-compose is running
		isRunning := client.IsRunning()

		// Build process cards
		var processCards []h.H
		for _, p := range processes {
			statusColor := getStatusColor(p.Status)
			processCards = append(processCards,
				h.Div(
					h.Style("border-bottom: 1px solid var(--pico-muted-border-color);"),
					// Process header row
					h.Div(
						h.Style("display: flex; justify-content: space-between; align-items: center; padding: 0.75rem 1rem;"),
						h.Div(
							h.Div(
								h.Style("display: flex; align-items: center; gap: 0.5rem;"),
								h.Span(
									h.Style(fmt.Sprintf("width: 10px; height: 10px; border-radius: 50%%; background-color: %s; display: inline-block;", statusColor)),
								),
								h.Strong(h.Text(p.Name)),
							),
							h.Div(
								h.Small(
									h.Style("color: var(--pico-muted-color);"),
									h.Text(fmt.Sprintf("Status: %s", p.Status)),
									h.If(p.PID > 0, h.Text(fmt.Sprintf(" | PID: %d", p.PID))),
									h.If(p.Restarts > 0, h.Text(fmt.Sprintf(" | Restarts: %d", p.Restarts))),
								),
							),
						),
						h.Div(
							h.Style("display: flex; gap: 0.25rem;"),
							h.If(p.IsRunning,
								h.Button(
									h.Class("secondary outline"),
									h.Style("padding: 0.25rem 0.5rem; font-size: 0.8rem;"),
									h.Text("Stop"),
									h.Attr("data-process", p.Name),
									h.Attr("onclick", fmt.Sprintf("fetch('http://localhost:%d/process/stop/%s', {method: 'PATCH'}).then(() => location.reload())", cfg.ProcessComposePort, p.Name)),
								),
							),
							h.If(!p.IsRunning && p.Status != "Running",
								h.Button(
									h.Style("padding: 0.25rem 0.5rem; font-size: 0.8rem;"),
									h.Text("Start"),
									h.Attr("data-process", p.Name),
									h.Attr("onclick", fmt.Sprintf("fetch('http://localhost:%d/process/start/%s', {method: 'POST'}).then(() => location.reload())", cfg.ProcessComposePort, p.Name)),
								),
							),
							h.Button(
								h.Class("contrast outline"),
								h.Style("padding: 0.25rem 0.5rem; font-size: 0.8rem;"),
								h.Text("Restart"),
								h.Attr("data-process", p.Name),
								h.Attr("onclick", fmt.Sprintf("fetch('http://localhost:%d/process/restart/%s', {method: 'POST'}).then(() => location.reload())", cfg.ProcessComposePort, p.Name)),
							),
						),
					),
					// Expandable logs panel
					h.Details(
						h.Style("margin: 0; padding: 0 1rem 0.75rem 1rem;"),
						h.Attr("data-process-logs", p.Name),
						h.Summary(
							h.Style("cursor: pointer; font-size: 0.85rem; color: var(--pico-muted-color);"),
							h.Attr("onclick", fmt.Sprintf("loadLogs('%s')", p.Name)),
							h.Text("View Logs"),
						),
						h.Pre(
							h.Attr("id", fmt.Sprintf("logs-%s", p.Name)),
							h.Style("background: #1e1e1e; color: #d4d4d4; padding: 0.75rem; border-radius: 0.5rem; font-size: 0.8rem; max-height: 300px; overflow: auto; margin-top: 0.5rem;"),
							h.Text("Click to load logs..."),
						),
					),
				),
			)
		}

		return h.Div(
			// Header with tabs
			viaNavHeader("processes", cfg),

			// Main content
			h.Main(
				h.Class("container"),
				h.Article(
					h.Div(
						h.Style("display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;"),
						h.H3(h.Text("Processes")),
						h.Div(
							h.Style("display: flex; align-items: center; gap: 1rem;"),
							h.Small(
								h.Style("color: var(--pico-muted-color);"),
								h.Text(lastRefresh.String()),
							),
							// Auto-refresh toggle
							h.Label(
								h.Style("display: flex; align-items: center; gap: 0.5rem; margin: 0; cursor: pointer;"),
								h.Input(
									h.Attr("type", "checkbox"),
									h.Attr("role", "switch"),
									h.If(autoRefresh.String() == "true", h.Attr("checked", "checked")),
									toggleAutoRefresh.OnClick(),
								),
								h.Small(h.Text("Auto")),
							),
							h.Button(
								h.Text("Refresh"),
								h.If(refreshing.String() == "true", h.Attr("aria-busy", "true")),
								h.If(refreshing.String() == "true", h.Attr("disabled", "disabled")),
								refreshAction.OnClick(),
							),
						),
					),

					// Status indicator
					h.If(!isRunning,
						h.Div(
							h.Style("background-color: #fff3cd; border: 1px solid #ffc107; border-radius: 0.5rem; padding: 1rem; margin-bottom: 1rem;"),
							h.Strong(h.Text("Process Compose not running")),
							h.P(
								h.Style("margin: 0.5rem 0 0 0;"),
								h.Text("Start process-compose with: "),
								h.Code(h.Text("xplat process up -D")),
								h.Text(" or "),
								h.Code(h.Text("xplat dev up")),
							),
						),
					),

					// Error message
					h.If(errorMsg.String() != "" && isRunning,
						h.Div(
							h.Style("background-color: #f8d7da; border: 1px solid #dc3545; border-radius: 0.5rem; padding: 1rem; margin-bottom: 1rem;"),
							h.Text(errorMsg.String()),
						),
					),

					// Process list
					h.If(len(processCards) > 0,
						h.Div(processCards...),
					),
					h.If(len(processCards) == 0 && isRunning && errorMsg.String() == "",
						h.P(
							h.Style("text-align: center; color: var(--pico-muted-color); padding: 2rem;"),
							h.Text("No processes found. Make sure process-compose is running."),
						),
					),
				),
			),

			// JavaScript for expandable logs panels
			h.Script(
				h.Raw(`
function loadLogs(processName) {
	const logEl = document.getElementById('logs-' + processName);
	if (!logEl) return;

	logEl.textContent = 'Loading...';

	fetch('/api/process/logs/' + processName)
		.then(r => r.json())
		.then(data => {
			logEl.textContent = data.logs || '(no logs available)';
			logEl.scrollTop = logEl.scrollHeight;
		})
		.catch(err => {
			logEl.textContent = 'Error fetching logs: ' + err.message;
		});
}
`),
			),
		)
	})
}
