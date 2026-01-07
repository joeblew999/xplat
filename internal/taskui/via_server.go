// Package taskui provides a web-based UI for running Taskfile tasks.
//
// This file implements the Via/Datastar version using SSE for bidirectional
// communication instead of WebSockets.
//
// Inspired by github.com/titpetric/task-ui (GPL-3.0 license).
// Original project: https://github.com/titpetric/task-ui
package taskui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-via/via"
	"github.com/go-via/via-plugin-picocss/picocss"
	"github.com/go-via/via/h"
)

// TaskInfo holds information about a task.
type TaskInfo struct {
	Name        string
	Description string
	Summary     string
}

// ViaConfig holds the Via server configuration.
type ViaConfig struct {
	Port        string // Port to listen on (default "3000")
	Taskfile    string // Path to Taskfile.yml (default "Taskfile.yml")
	WorkDir     string // Working directory for task execution
	OpenBrowser bool   // Open browser on start
}

// DefaultViaConfig returns sensible defaults.
func DefaultViaConfig() ViaConfig {
	return ViaConfig{
		Port:        "3000",
		Taskfile:    "Taskfile.yml",
		WorkDir:     "",
		OpenBrowser: true,
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

	// Index page - task list
	v.Page("/", func(c *via.Context) {
		viaTaskListPage(c, tasks, cfg)
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
			// Header
			h.Nav(
				h.Style("background-color: #343a40; padding: 1rem; margin-bottom: 1rem;"),
				h.Div(
					h.Style("display: flex; justify-content: space-between; align-items: center; max-width: 1200px; margin: 0 auto;"),
					h.A(
						h.Href("/"),
						h.Style("color: white; text-decoration: none; font-size: 1.25rem;"),
						h.Strong(h.Text("xplat ")),
						h.Text("Task UI"),
					),
					h.Span(
						h.Style("color: #6c757d;"),
						h.Text(cfg.WorkDir),
					),
				),
			),

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
			// Header
			h.Nav(
				h.Style("background-color: #343a40; padding: 1rem; margin-bottom: 1rem;"),
				h.Div(
					h.Style("display: flex; justify-content: space-between; align-items: center; max-width: 1200px; margin: 0 auto;"),
					h.A(
						h.Href("/"),
						h.Style("color: white; text-decoration: none; font-size: 1.25rem;"),
						h.Strong(h.Text("xplat ")),
						h.Text("Task UI"),
					),
					h.Span(
						h.Style("color: #6c757d;"),
						h.Text(cfg.WorkDir),
					),
				),
			),

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
