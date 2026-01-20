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
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-via/via"
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
	Port               string // Port to listen on (default "3000")
	Taskfile           string // Path to Taskfile.yml (default "Taskfile.yml")
	WorkDir            string // Working directory for task execution
	OpenBrowser        bool   // Open browser on start
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
					h.Href("/tasks/"+t.Name),
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
			RenderNav("tasks", cfg.WorkDir),

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
					h.Href("/tasks/"+t.Name),
					h.Style(style),
					h.Text(displayName),
				)
				if isActive {
					link = h.A(
						h.Href("/tasks/"+t.Name),
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
			RenderNav("tasks", cfg.WorkDir),

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

// RenderNav is the single source of truth for navigation.
// Used by both app.go and the page functions in this file.
func RenderNav(activeTab string, workDir string) h.H {
	tabStyle := func(tab string) string {
		base := "color: white; text-decoration: none; padding: 0.5rem 1rem; border-radius: 0.25rem 0.25rem 0 0;"
		if tab == activeTab {
			return base + " background-color: #495057;"
		}
		return base
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
					h.Strong(h.Text("xplat")),
				),
				h.Div(
					h.Style("display: flex; gap: 0.25rem; margin-left: 1rem;"),
					h.A(
						h.Href("/"),
						h.Style(tabStyle("home")),
						h.Text("Home"),
					),
					h.A(
						h.Href("/tasks"),
						h.Style(tabStyle("tasks")),
						h.Text("Tasks"),
					),
					h.A(
						h.Href("/processes"),
						h.Style(tabStyle("processes")),
						h.Text("Processes"),
					),
					h.A(
						h.Href("/setup"),
						h.Style(tabStyle("setup")),
						h.Text("Setup"),
					),
				),
			),
			h.Span(
				h.Style("color: #6c757d;"),
				h.Text(workDir),
			),
		),
	)
}

// viaProcessListPage renders the process-compose status page with tabs.
func viaProcessListPage(c *via.Context, client *ProcessComposeClient, cfg ViaConfig) {
	// Signals for state management
	autoRefresh := c.Signal(true) // Auto-refresh enabled by default
	activeTab := c.Signal("status") // status, logs, graph

	// Toggle auto-refresh action
	toggleAutoRefresh := c.Action(func() {
		if autoRefresh.String() == "true" {
			autoRefresh.SetValue(false)
		} else {
			autoRefresh.SetValue(true)
		}
		c.Sync()
	})

	// Refresh action - just triggers a re-render
	refreshAction := c.Action(func() {
		c.Sync()
	})

	// Tab switch actions
	switchToStatus := c.Action(func() {
		activeTab.SetValue("status")
		c.Sync()
	})
	switchToLogs := c.Action(func() {
		activeTab.SetValue("logs")
		c.Sync()
	})
	switchToGraph := c.Action(func() {
		activeTab.SetValue("graph")
		c.Sync()
	})

	// NOTE: Auto-refresh is handled client-side via JavaScript setInterval
	// to avoid goroutine leaks from server-side tickers

	c.View(func() h.H {
		// Fetch processes on each render (synchronous, no goroutine leak)
		var processes []ProcessInfo
		var errorMsg string
		fetchedProcesses, err := client.ListProcesses()
		if err != nil {
			errorMsg = err.Error()
		} else {
			processes = fetchedProcesses
		}
		lastRefresh := fmt.Sprintf("Last updated: %s", time.Now().Format("15:04:05"))

		// Check if process-compose is running
		isRunning := client.IsRunning()

		// Tab button style helper
		tabStyle := func(tab string) string {
			base := "padding: 0.5rem 1rem; border: none; background: none; cursor: pointer; border-bottom: 2px solid transparent; margin-right: 0.5rem;"
			if activeTab.String() == tab {
				return base + " border-bottom-color: var(--pico-primary); font-weight: bold;"
			}
			return base + " color: var(--pico-muted-color);"
		}

		// Build process names for filter dropdown
		var processNames []string
		for _, p := range processes {
			processNames = append(processNames, p.Name)
		}

		// Build process cards for status tab
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
									h.Attr("onclick", fmt.Sprintf("fetch('/api/process/stop/%s', {method: 'POST'}).then(() => location.reload())", p.Name)),
								),
							),
							h.If(!p.IsRunning && p.Status != "Running",
								h.Button(
									h.Style("padding: 0.25rem 0.5rem; font-size: 0.8rem;"),
									h.Text("Start"),
									h.Attr("data-process", p.Name),
									h.Attr("onclick", fmt.Sprintf("fetch('/api/process/start/%s', {method: 'POST'}).then(() => location.reload())", p.Name)),
								),
							),
							h.Button(
								h.Class("contrast outline"),
								h.Style("padding: 0.25rem 0.5rem; font-size: 0.8rem;"),
								h.Text("Restart"),
								h.Attr("data-process", p.Name),
								h.Attr("onclick", fmt.Sprintf("fetch('/api/process/restart/%s', {method: 'POST'}).then(() => location.reload())", p.Name)),
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

		// Build process filter options for logs tab
		var filterOptions []h.H
		filterOptions = append(filterOptions,
			h.Attr("id", "log-filter"),
			h.Style("margin: 0; padding: 0.25rem 0.5rem;"),
			h.Attr("onchange", "filterLogs(this.value)"),
			h.Option(h.Attr("value", "all"), h.Text("All Processes")),
		)
		for _, name := range processNames {
			filterOptions = append(filterOptions, h.Option(h.Attr("value", name), h.Text(name)))
		}

		// Build process names JSON for JavaScript
		processNamesJSON := "[]"
		if len(processNames) > 0 {
			var quoted []string
			for _, name := range processNames {
				quoted = append(quoted, fmt.Sprintf("%q", name))
			}
			processNamesJSON = "[" + strings.Join(quoted, ",") + "]"
		}

		return h.Div(
			// Hidden element with process names for JavaScript
			h.Attr("data-process-names", processNamesJSON),

			// Header with tabs
			RenderNav("processes", cfg.WorkDir),

			// Main content
			h.Main(
				h.Class("container"),
				h.Article(
					// Header with title and controls
					h.Div(
						h.Style("display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;"),
						h.H3(h.Style("margin: 0;"), h.Text("Processes")),
						h.Div(
							h.Style("display: flex; align-items: center; gap: 1rem;"),
							h.Small(
								h.Style("color: var(--pico-muted-color);"),
								h.Text(lastRefresh),
							),
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
								refreshAction.OnClick(),
							),
						),
					),

					// Tab navigation
					h.Div(
						h.Style("border-bottom: 1px solid var(--pico-muted-border-color); margin-bottom: 1rem;"),
						h.Button(
							h.Style(tabStyle("status")),
							h.Text("Status"),
							switchToStatus.OnClick(),
						),
						h.Button(
							h.Style(tabStyle("logs")),
							h.Text("Logs"),
							switchToLogs.OnClick(),
						),
						h.Button(
							h.Style(tabStyle("graph")),
							h.Text("Graph"),
							switchToGraph.OnClick(),
						),
					),

					// Status indicator (shown on all tabs)
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
					h.If(errorMsg != "" && isRunning,
						h.Div(
							h.Style("background-color: #f8d7da; border: 1px solid #dc3545; border-radius: 0.5rem; padding: 1rem; margin-bottom: 1rem;"),
							h.Text(errorMsg),
						),
					),

					// STATUS TAB
					h.If(activeTab.String() == "status",
						h.Div(
							h.If(len(processCards) > 0,
								h.Div(processCards...),
							),
							h.If(len(processCards) == 0 && isRunning && errorMsg == "",
								h.P(
									h.Style("text-align: center; color: var(--pico-muted-color); padding: 2rem;"),
									h.Text("No processes found. Make sure process-compose is running."),
								),
							),
						),
					),

					// LOGS TAB
					h.If(activeTab.String() == "logs",
						h.Div(
							// Filter controls
							h.Div(
								h.Style("display: flex; gap: 1rem; align-items: center; margin-bottom: 1rem;"),
								h.Label(
									h.Style("display: flex; align-items: center; gap: 0.5rem; margin: 0;"),
									h.Text("Filter: "),
									h.Select(filterOptions...),
								),
								h.Button(
									h.Class("outline"),
									h.Style("padding: 0.25rem 0.75rem;"),
									h.Text("Refresh Logs"),
									h.Attr("onclick", "refreshAllLogs()"),
								),
								h.Label(
									h.Style("display: flex; align-items: center; gap: 0.5rem; margin: 0; margin-left: auto;"),
									h.Input(
										h.Attr("type", "checkbox"),
										h.Attr("id", "logs-auto-scroll"),
										h.Attr("checked", "checked"),
									),
									h.Small(h.Text("Auto-scroll")),
								),
							),
							// Combined logs display
							h.Pre(
								h.Attr("id", "combined-logs"),
								h.Style("background: #1e1e1e; color: #d4d4d4; padding: 1rem; border-radius: 0.5rem; font-size: 0.8rem; height: 500px; overflow: auto; font-family: 'Menlo', 'Monaco', 'Courier New', monospace;"),
								h.Text("Loading logs..."),
							),
						),
					),

					// GRAPH TAB
					h.If(activeTab.String() == "graph",
						h.Div(
							// Graph format selector
							h.Div(
								h.Style("display: flex; gap: 1rem; align-items: center; margin-bottom: 1rem;"),
								h.Label(
									h.Style("display: flex; align-items: center; gap: 0.5rem; margin: 0;"),
									h.Text("Format: "),
									h.Select(
										h.Attr("id", "graph-format"),
										h.Style("margin: 0; padding: 0.25rem 0.5rem;"),
										h.Attr("onchange", "loadGraph(this.value)"),
										h.Option(h.Attr("value", "ascii"), h.Attr("selected", "selected"), h.Text("ASCII Tree")),
										h.Option(h.Attr("value", "mermaid"), h.Text("Mermaid Diagram")),
									),
								),
								h.Button(
									h.Class("outline"),
									h.Style("padding: 0.25rem 0.75rem;"),
									h.Text("Refresh Graph"),
									h.Attr("onclick", "loadGraph(document.getElementById('graph-format').value)"),
								),
							),
							// Graph display
							h.Div(
								h.Attr("id", "graph-container"),
								h.Pre(
									h.Attr("id", "graph-display"),
									h.Style("background: #1e1e1e; color: #d4d4d4; padding: 1rem; border-radius: 0.5rem; font-size: 0.9rem; min-height: 300px; overflow: auto; font-family: 'Menlo', 'Monaco', 'Courier New', monospace;"),
									h.Text("Loading dependency graph..."),
								),
							),
							// Mermaid container (hidden by default)
							h.Div(
								h.Attr("id", "mermaid-container"),
								h.Style("display: none; background: white; padding: 1rem; border-radius: 0.5rem; min-height: 300px;"),
							),
						),
					),
				),
			),

			// JavaScript for logs, graph, and process controls
			h.Script(
				h.Raw(`
// Process names for combined logs (fetched from filter dropdown - no extra HTTP request)
var processNames = [];
var allLogs = {};
var currentFilter = 'all';
var logsLoading = false;  // Prevent concurrent loads
var graphLoading = false; // Prevent concurrent loads

// Colors for different processes
var processColors = {
	'db': '#4CAF50',
	'cache': '#2196F3',
	'api': '#FF9800',
	'web': '#9C27B0',
	'worker-email': '#00BCD4',
	'worker-jobs': '#E91E63',
	'backup': '#795548',
	'cleanup': '#607D8B',
};

function getProcessColor(name) {
	if (processColors[name]) return processColors[name];
	// Generate color from hash for unknown processes
	var hash = 0;
	for (var i = 0; i < name.length; i++) {
		hash = name.charCodeAt(i) + ((hash << 5) - hash);
	}
	var h = hash % 360;
	return 'hsl(' + h + ', 70%, 60%)';
}

// Load logs for a single process (for status tab expandable panels)
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

// Get process names from data attribute (always available regardless of tab)
function getProcessNames() {
	// First try data attribute on the main container
	var container = document.querySelector('[data-process-names]');
	if (container) {
		try {
			return JSON.parse(container.getAttribute('data-process-names')) || [];
		} catch (e) {
			console.error('Failed to parse process names:', e);
		}
	}
	// Fallback: try dropdown if visible
	var select = document.getElementById('log-filter');
	if (select) {
		var names = [];
		for (var i = 0; i < select.options.length; i++) {
			var val = select.options[i].value;
			if (val !== 'all') {
				names.push(val);
			}
		}
		return names;
	}
	return [];
}

// Fetch logs for all processes
function refreshAllLogs() {
	if (logsLoading) return; // Prevent concurrent loads

	const logEl = document.getElementById('combined-logs');
	if (!logEl) return;

	logsLoading = true;
	logEl.textContent = 'Loading logs...';

	// Get process names from data attribute (always available)
	processNames = getProcessNames();
	allLogs = {};

	if (processNames.length === 0) {
		logEl.textContent = '(no processes - is process-compose running?)';
		logsLoading = false;
		return;
	}

	var pending = processNames.length;
	processNames.forEach(function(name) {
		fetch('/api/process/logs/' + name)
			.then(r => r.json())
			.then(data => {
				allLogs[name] = (data.logs || '').split('\n').filter(l => l.trim());
				pending--;
				if (pending === 0) {
					renderCombinedLogs();
					logsLoading = false;
				}
			})
			.catch(err => {
				allLogs[name] = ['Error: ' + err.message];
				pending--;
				if (pending === 0) {
					renderCombinedLogs();
					logsLoading = false;
				}
			});
	});
}

// Render combined logs with color coding
function renderCombinedLogs() {
	const logEl = document.getElementById('combined-logs');
	if (!logEl) return;

	var lines = [];

	if (currentFilter === 'all') {
		// Interleave all logs with process prefix
		Object.keys(allLogs).forEach(function(name) {
			var color = getProcessColor(name);
			allLogs[name].forEach(function(line) {
				lines.push({
					process: name,
					color: color,
					text: line,
					// Try to extract timestamp for sorting
					time: extractTime(line)
				});
			});
		});
		// Sort by timestamp if available
		lines.sort(function(a, b) {
			return a.time.localeCompare(b.time);
		});
	} else {
		// Single process logs
		var logs = allLogs[currentFilter] || [];
		var color = getProcessColor(currentFilter);
		logs.forEach(function(line) {
			lines.push({
				process: currentFilter,
				color: color,
				text: line,
				time: extractTime(line)
			});
		});
	}

	// Build HTML with colored prefixes
	logEl.innerHTML = '';
	lines.forEach(function(line) {
		var span = document.createElement('span');
		if (currentFilter === 'all') {
			var prefix = document.createElement('span');
			prefix.style.color = line.color;
			prefix.style.fontWeight = 'bold';
			prefix.textContent = '[' + line.process + '] ';
			span.appendChild(prefix);
		}
		span.appendChild(document.createTextNode(line.text + '\n'));
		logEl.appendChild(span);
	});

	// Auto-scroll to bottom
	var autoScroll = document.getElementById('logs-auto-scroll');
	if (autoScroll && autoScroll.checked) {
		logEl.scrollTop = logEl.scrollHeight;
	}
}

function extractTime(line) {
	// Try to extract HH:MM:SS timestamp
	var match = line.match(/(\d{2}:\d{2}:\d{2})/);
	return match ? match[1] : '00:00:00';
}

function filterLogs(value) {
	currentFilter = value;
	renderCombinedLogs();
}

// Graph loading
function loadGraph(format) {
	if (graphLoading) return; // Prevent concurrent loads

	const graphEl = document.getElementById('graph-display');
	const graphContainer = document.getElementById('graph-container');
	const mermaidContainer = document.getElementById('mermaid-container');

	if (!graphEl) return;

	graphLoading = true;

	// Show appropriate container
	if (format === 'mermaid') {
		graphContainer.style.display = 'none';
		mermaidContainer.style.display = 'block';
		mermaidContainer.innerHTML = '<div style="text-align: center; padding: 2rem;">Loading Mermaid diagram...</div>';
	} else {
		graphContainer.style.display = 'block';
		mermaidContainer.style.display = 'none';
		graphEl.textContent = 'Loading graph...';
	}

	fetch('/api/process/graph?format=' + format)
		.then(r => r.text())
		.then(data => {
			graphLoading = false;
			if (format === 'mermaid') {
				renderMermaidGraph(data);
			} else {
				// For ASCII and other formats, render the tree from JSON
				renderAsciiGraph(JSON.parse(data));
			}
		})
		.catch(err => {
			graphLoading = false;
			if (format === 'mermaid') {
				mermaidContainer.innerHTML = '<div style="color: red;">Error: ' + err.message + '</div>';
			} else {
				graphEl.textContent = 'Error loading graph: ' + err.message;
			}
		});
}

function renderAsciiGraph(data) {
	const graphEl = document.getElementById('graph-display');
	if (!graphEl || !data.nodes) {
		graphEl.textContent = 'No dependency data available';
		return;
	}

	var output = 'Dependency Graph\n';
	output += '================\n\n';
	var nodes = data.nodes;
	var nodeNames = Object.keys(nodes);

	// Recursive function to render node and its dependencies
	function renderNode(node, indent, isLast, depType) {
		var prefix = indent + (isLast ? '└── ' : '├── ');
		var status = node.process_status || 'Unknown';
		var statusIcon = status === 'Running' ? '🟢' : (status === 'Disabled' ? '⚫' : '🟡');
		var depLabel = depType ? ' <' + depType + '>' : '';

		output += prefix + statusIcon + ' ' + node.name + depLabel + ' [' + status + ']\n';

		// Render dependencies recursively
		if (node.depends_on) {
			var deps = Object.keys(node.depends_on);
			var childIndent = indent + (isLast ? '    ' : '│   ');
			deps.forEach(function(depName, depIdx) {
				var dep = node.depends_on[depName];
				var depIsLast = depIdx === deps.length - 1;
				renderNode(dep, childIndent, depIsLast, dep.dependency_type);
			});
		}
	}

	// Render all root nodes
	nodeNames.forEach(function(name, idx) {
		var node = nodes[name];
		var isLast = idx === nodeNames.length - 1;
		renderNode(node, '', isLast, null);
	});

	graphEl.textContent = output;
}

function renderMermaidGraph(data) {
	const container = document.getElementById('mermaid-container');

	try {
		var graphData = JSON.parse(data);
		var mermaidCode = 'flowchart TD\n';
		var edges = [];

		// Recursive function to collect all edges
		function collectEdges(node, parentName) {
			if (node.depends_on) {
				Object.keys(node.depends_on).forEach(function(depName) {
					var dep = node.depends_on[depName];
					var fromNode = node.name.replace(/-/g, '_');
					var toNode = dep.name.replace(/-/g, '_');
					var edge = fromNode + ' --> ' + toNode;
					if (edges.indexOf(edge) === -1) {
						edges.push(edge);
					}
					collectEdges(dep, dep.name);
				});
			}
		}

		// Build mermaid diagram from JSON (recursive)
		var nodes = graphData.nodes || {};
		Object.keys(nodes).forEach(function(name) {
			collectEdges(nodes[name], name);
		});

		edges.forEach(function(edge) {
			mermaidCode += '    ' + edge + '\n';
		});

		// Display mermaid code with styling info
		container.innerHTML = '<div style="padding: 1rem;">' +
			'<p style="margin-bottom: 1rem; color: #666;">Mermaid diagram code (copy to <a href="https://mermaid.live" target="_blank">mermaid.live</a> to visualize):</p>' +
			'<pre style="background: #f5f5f5; padding: 1rem; border-radius: 0.5rem; font-size: 0.85rem;">' + mermaidCode + '</pre>' +
			'</div>';
	} catch (e) {
		container.innerHTML = '<div style="color: red;">Error parsing graph data: ' + e.message + '</div>';
	}
}

// Initialize tabs - called once when page loads
var tabsInitialized = false;
function initializeTabs() {
	if (tabsInitialized) return;
	tabsInitialized = true;

	// Initial load if on logs tab
	var logsEl = document.getElementById('combined-logs');
	if (logsEl && logsEl.textContent === 'Loading logs...') {
		setTimeout(refreshAllLogs, 100);
	}
	// Initial load if on graph tab
	var graphEl = document.getElementById('graph-display');
	if (graphEl && graphEl.textContent === 'Loading dependency graph...') {
		setTimeout(function() { loadGraph('ascii'); }, 100);
	}
}

document.addEventListener('DOMContentLoaded', initializeTabs);

// Watch for tab switches (Via re-renders content)
// Use a debounced observer to prevent rapid re-triggers
var observerDebounce = null;
var observer = new MutationObserver(function(mutations) {
	if (observerDebounce) return;
	observerDebounce = setTimeout(function() {
		observerDebounce = null;
		var logsEl = document.getElementById('combined-logs');
		if (logsEl && logsEl.textContent === 'Loading logs...' && !logsLoading) {
			refreshAllLogs();
		}
		var graphEl = document.getElementById('graph-display');
		if (graphEl && graphEl.textContent === 'Loading dependency graph...' && !graphLoading) {
			loadGraph('ascii');
		}
	}, 200);
});
observer.observe(document.body, { childList: true, subtree: true });

// Note: Auto-refresh for Status tab is handled by Via's built-in SSE mechanism
// The Refresh button triggers a c.Sync() which updates all process data
// No need for client-side polling that could cause browser lockups
`),
			),
		)
	})
}
