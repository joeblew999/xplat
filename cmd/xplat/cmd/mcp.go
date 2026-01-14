// Package cmd provides CLI commands for xplat.
//
// mcp.go - Embedded MCP (Model Context Protocol) server
//
// This provides an MCP server that exposes Taskfile tasks as MCP tools,
// allowing AI assistants (Claude Desktop, Cursor, etc.) to execute tasks.
//
// Based on: https://github.com/rsclarke/mcp-taskfile-server
// Uses: https://github.com/mark3labs/mcp-go
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/go-task/task/v3"
	"github.com/go-task/task/v3/experiments"
	"github.com/go-task/task/v3/taskfile/ast"
	"github.com/joeblew999/xplat/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCP environment variable for port override
const envMCPPort = "XPLAT_MCP_PORT"

// MCPCmd is the parent command for MCP operations
var MCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP (Model Context Protocol) server",
	Long: `Provides an MCP server that exposes Taskfile tasks as tools for AI assistants.

This allows AI IDEs like Claude Desktop, Cursor, Windsurf, etc. to discover
and execute your Taskfile tasks directly.

Examples:
  xplat mcp serve              # Start MCP server (stdio)
  xplat mcp list               # List tasks that would be exposed
  xplat mcp config             # Show config for AI IDEs`,
}

// MCPServeCmd starts the MCP server
var MCPServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server (stdio or HTTP transport)",
	Long: `Starts an MCP server that exposes all Taskfile tasks as MCP tools.

TRANSPORTS:
  stdio (default)  - JSON-RPC over stdin/stdout, spawned by AI client
  http             - HTTP server on specified port, runs as a service

STDIO MODE (default):
  xplat mcp serve

  Config for Claude Desktop/Cursor/Crush:
    {
      "mcpServers": {
        "xplat": {
          "type": "stdio",
          "command": "xplat",
          "args": ["mcp", "serve"],
          "cwd": "/path/to/your/project"
        }
      }
    }

HTTP MODE:
  xplat mcp serve --http           # Uses default port :8765
  xplat mcp serve --http :9000     # Custom port

  Environment: XPLAT_MCP_PORT=9000

  Config for Crush (http transport):
    {
      "mcp": {
        "xplat": {
          "type": "http",
          "url": "http://localhost:8765/mcp"
        }
      }
    }

  Can run in process-compose as a persistent service.`,
	RunE: runMCPServe,
}

// MCPListCmd lists tasks that would be exposed
var MCPListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks that would be exposed as MCP tools",
	RunE:  runMCPList,
}

// MCPConfigCmd shows configuration for AI IDEs
var MCPConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show MCP configuration for AI IDEs",
	RunE:  runMCPConfig,
}

var (
	mcpDir      string
	mcpTaskfile string
	mcpHTTP     string
	mcpSSE      string
)

func init() {
	MCPCmd.AddCommand(MCPServeCmd)
	MCPCmd.AddCommand(MCPListCmd)
	MCPCmd.AddCommand(MCPConfigCmd)

	MCPServeCmd.Flags().StringVarP(&mcpDir, "dir", "d", "", "Working directory")
	MCPServeCmd.Flags().StringVarP(&mcpTaskfile, "taskfile", "t", "", "Taskfile to use")
	MCPServeCmd.Flags().StringVar(&mcpHTTP, "http", "", "HTTP address (default :"+config.DefaultMCPPort+", or $XPLAT_MCP_PORT)")
	MCPServeCmd.Flags().StringVar(&mcpSSE, "sse", "", "SSE address (default :"+config.DefaultMCPPort+", or $XPLAT_MCP_PORT)")

	MCPListCmd.Flags().StringVarP(&mcpDir, "dir", "d", "", "Working directory")
	MCPListCmd.Flags().StringVarP(&mcpTaskfile, "taskfile", "t", "", "Taskfile to use")
}

// taskfileServer wraps the task executor for MCP
type taskfileServer struct {
	executor *task.Executor
	taskfile *ast.Taskfile
	workdir  string
}

// newTaskfileServer creates a new taskfile server
func newTaskfileServer(dir, entrypoint string) (*taskfileServer, error) {
	workdir := dir
	if workdir == "" {
		var err error
		workdir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Set PLAT_* env vars like xplat task does
	config.SetPlatEnv(workdir)
	os.Setenv("PATH", config.PathWithPlatBin(workdir))

	// Enable remote taskfiles
	os.Setenv("TASK_X_REMOTE_TASKFILES", "1")
	experiments.Parse(workdir)

	// Create executor options
	opts := []task.ExecutorOption{
		task.WithDir(workdir),
		task.WithSilent(true),
	}

	executor := task.NewExecutor(opts...)

	if entrypoint != "" {
		executor.Entrypoint = entrypoint
	}

	// Setup the executor (loads Taskfile)
	if err := executor.Setup(); err != nil {
		return nil, fmt.Errorf("failed to setup task executor: %w", err)
	}

	return &taskfileServer{
		executor: executor,
		taskfile: executor.Taskfile,
		workdir:  workdir,
	}, nil
}

// createTaskHandler creates a handler function for a specific task
func (s *taskfileServer) createTaskHandler(taskName string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract variables from request arguments
		arguments := request.GetArguments()
		vars := ast.NewVars()

		// Add all provided arguments as variables
		for key, value := range arguments {
			if strValue, ok := value.(string); ok {
				vars.Set(key, ast.Var{Value: strValue})
			}
		}

		// Create buffers to capture output
		var stdout, stderr bytes.Buffer

		// Create a new executor with output capture
		executor := task.NewExecutor(
			task.WithDir(s.workdir),
			task.WithStdout(&stdout),
			task.WithStderr(&stderr),
			task.WithSilent(true),
		)

		// Setup the executor
		if err := executor.Setup(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Task '%s' setup failed: %v", taskName, err)), nil
		}

		// Create a call for this task
		call := &task.Call{
			Task: taskName,
			Vars: vars,
		}

		// Execute the task
		err := executor.Run(ctx, call)

		// Collect output
		stdoutStr := stdout.String()
		stderrStr := stderr.String()

		// Build result message
		var result strings.Builder

		if err != nil {
			result.WriteString(fmt.Sprintf("Task '%s' failed with error: %v\n", taskName, err))
		} else {
			result.WriteString(fmt.Sprintf("Task '%s' completed successfully.\n", taskName))
		}

		if stdoutStr != "" {
			result.WriteString(fmt.Sprintf("\nOutput:\n%s", stdoutStr))
		}

		if stderrStr != "" {
			result.WriteString(fmt.Sprintf("\nErrors:\n%s", stderrStr))
		}

		if err != nil {
			return mcp.NewToolResultError(result.String()), nil
		}

		return mcp.NewToolResultText(result.String()), nil
	}
}

// createToolForTask creates an MCP tool definition for a task
func (s *taskfileServer) createToolForTask(taskName string, taskDef *ast.Task) mcp.Tool {
	description := taskDef.Desc
	if description == "" {
		description = fmt.Sprintf("Execute task: %s", taskName)
	}

	toolOptions := []mcp.ToolOption{
		mcp.WithDescription(description),
	}

	// Collect all variables (global + task-specific)
	allVars := make(map[string]ast.Var)

	// Add global variables
	if s.taskfile.Vars != nil && s.taskfile.Vars.Len() > 0 {
		for varName, varDef := range s.taskfile.Vars.All() {
			allVars[varName] = varDef
		}
	}

	// Add task-specific variables (override global)
	if taskDef.Vars != nil && taskDef.Vars.Len() > 0 {
		for varName, varDef := range taskDef.Vars.All() {
			allVars[varName] = varDef
		}
	}

	// Add parameters for all variables
	for varName, varDef := range allVars {
		defaultValue := ""
		if strVal, ok := varDef.Value.(string); ok {
			defaultValue = strVal
		}

		toolOptions = append(toolOptions,
			mcp.WithString(varName,
				mcp.Description(fmt.Sprintf("Variable: %s (default: %s)", varName, defaultValue)),
			),
		)
	}

	return mcp.NewTool(taskName, toolOptions...)
}

// registerTasks registers all tasks as MCP tools
func (s *taskfileServer) registerTasks(mcpServer *server.MCPServer) error {
	if s.taskfile.Tasks == nil {
		return fmt.Errorf("no tasks found in Taskfile")
	}

	for taskName, taskDef := range s.taskfile.Tasks.All(nil) {
		// Skip internal tasks
		if strings.HasPrefix(taskName, ":") {
			continue
		}

		tool := s.createToolForTask(taskName, taskDef)
		handler := s.createTaskHandler(taskName)
		mcpServer.AddTool(tool, handler)
	}

	return nil
}

// getTasks returns all task names
func (s *taskfileServer) getTasks() []string {
	var tasks []string
	if s.taskfile.Tasks != nil {
		for taskName := range s.taskfile.Tasks.All(nil) {
			if !strings.HasPrefix(taskName, ":") {
				tasks = append(tasks, taskName)
			}
		}
	}
	return tasks
}

// getMCPHTTPAddress returns the HTTP address for MCP server.
// Priority: flag value > env var > default port
func getMCPHTTPAddress(flagValue string) string {
	// If flag has an explicit value, use it
	if flagValue != "" && flagValue != "true" {
		return flagValue
	}

	// Check environment variable
	if envPort := os.Getenv(envMCPPort); envPort != "" {
		if !strings.HasPrefix(envPort, ":") {
			return ":" + envPort
		}
		return envPort
	}

	// Use default port from config
	return ":" + config.DefaultMCPPort
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	// Create taskfile server
	tfServer, err := newTaskfileServer(mcpDir, mcpTaskfile)
	if err != nil {
		return fmt.Errorf("failed to create taskfile server: %w", err)
	}

	// Create MCP server with resource capabilities
	mcpServer := server.NewMCPServer(
		"xplat-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(true, false),
	)

	// Register all tasks as MCP tools
	if err := tfServer.registerTasks(mcpServer); err != nil {
		return fmt.Errorf("failed to register tasks: %w", err)
	}

	// Register xplat documentation as MCP resources
	registerXplatResources(mcpServer)

	// Check if --http flag was provided (with or without value)
	httpFlagProvided := cmd.Flags().Changed("http")

	// Choose transport based on flags
	switch {
	case httpFlagProvided || mcpHTTP != "":
		// HTTP transport - runs as a persistent service
		addr := getMCPHTTPAddress(mcpHTTP)
		fmt.Printf("Starting MCP HTTP server on %s\n", addr)
		fmt.Printf("Endpoint: http://localhost%s/mcp\n", addr)
		return server.NewStreamableHTTPServer(mcpServer).Start(addr)

	case mcpSSE != "":
		// SSE transport - Server-Sent Events
		addr := getMCPHTTPAddress(mcpSSE)
		fmt.Printf("Starting MCP SSE server on %s\n", addr)
		fmt.Printf("Endpoint: http://localhost%s/sse\n", addr)
		return server.NewSSEServer(mcpServer).Start(addr)

	default:
		// stdio transport (default) - spawned by AI client
		return server.ServeStdio(mcpServer)
	}
}

// registerXplatResources adds xplat documentation as MCP resources
func registerXplatResources(mcpServer *server.MCPServer) {
	// Resource: xplat OS utilities documentation
	osDocsResource := mcp.NewResource(
		"xplat://docs/os",
		"xplat OS Utilities",
		mcp.WithResourceDescription("Cross-platform OS utilities available in xplat. Use 'xplat os <command>' in Taskfiles."),
		mcp.WithMIMEType("text/plain"),
	)

	mcpServer.AddResource(osDocsResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		docs := `xplat OS Utilities - Cross-platform commands for Taskfiles

IMPORTANT: All OS commands are under 'xplat os', not directly under 'xplat'.

FILE OPERATIONS:
  xplat os cat <file>              Print file contents
  xplat os cp <src> <dst> [-r]     Copy files or directories
  xplat os mkdir -p <dir>          Create directories (with parents)
  xplat os mv <src> <dst>          Move or rename files
  xplat os rm [-rf] <path>         Remove files or directories
  xplat os touch <file>            Create file or update timestamp

ENVIRONMENT & TEXT:
  xplat os env <VAR>               Get environment variable
  xplat os envsubst                Substitute env vars in text
  xplat os glob "<pattern>"        Expand glob pattern
  xplat os jq <expr> <file>        Process JSON with jq syntax

ARCHIVES & DOWNLOADS:
  xplat os extract <archive>       Extract zip, tar.gz, etc.
  xplat os fetch <url> [-x]        Download file, optionally extract
    --output DIR                   Output directory
    --extract                      Extract archive after download
    --strip N                      Remove N path components

VERSION CONTROL:
  xplat os git <args>              Git operations (no git binary required)

TOOLS:
  xplat os which <binary>          Find binary in managed locations or PATH
  xplat os version-file            Read/write .version file

EXAMPLES IN TASKFILES:
  - cmd: xplat os mkdir -p "{{.PLAT_BIN}}"
  - cmd: xplat os fetch --extract "https://example.com/tool.tar.gz" --output "{{.PLAT_BIN}}"
  - cmd: xplat os rm -rf "{{.PLAT_DATA}}/cache"
  - cmd: xplat os cp -r src/ dst/
  - cmd: xplat os glob "**/*.go" | wc -l

WHY USE THESE?
These utilities work identically on macOS, Linux, and Windows.
They fill gaps in Task's shell interpreter for cross-platform Taskfiles.
`
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/plain",
				Text:     docs,
			},
		}, nil
	})

	// Resource: xplat command overview
	overviewResource := mcp.NewResource(
		"xplat://docs/overview",
		"xplat Command Overview",
		mcp.WithResourceDescription("Overview of all xplat commands and their purpose."),
		mcp.WithMIMEType("text/plain"),
	)

	mcpServer.AddResource(overviewResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		docs := `xplat - One binary to bootstrap and run any plat-* project

CORE COMMANDS:
  xplat task [task...]     Run Taskfile tasks (embedded Task runner)
  xplat process            Run services (embedded process-compose)
  xplat gen all            Generate files from xplat.yaml manifest
  xplat pkg install <pkg>  Install package from registry

OS UTILITIES (use 'xplat os <cmd>'):
  mkdir, rm, cp, mv, cat, touch, glob, fetch, extract, env, envsubst, jq, git, which

PACKAGE MANAGEMENT:
  xplat pkg list           List available packages
  xplat pkg install <pkg>  Install package (binary + taskfile)
  xplat binary install     Install binary from GitHub release

MCP SERVER:
  xplat mcp serve          Start MCP server (exposes Taskfile tasks)
  xplat mcp list           List exposed MCP tools
  xplat mcp config         Show config for AI IDEs

DOCUMENTATION:
  xplat docs readme        Generate README.md from commands
  xplat docs taskfile      Generate Taskfile.generated.yml

SYNC (external service monitoring):
  xplat sync-gh            GitHub sync (releases, CI, issues)
  xplat sync-cf            Cloudflare sync (deployments)

DIRECTORY CONVENTIONS:
  .bin/   (PLAT_BIN)       Downloaded/built binaries
  .data/  (PLAT_DATA)      Runtime data, caches, logs
`
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/plain",
				Text:     docs,
			},
		}, nil
	})
}

func runMCPList(cmd *cobra.Command, args []string) error {
	tfServer, err := newTaskfileServer(mcpDir, mcpTaskfile)
	if err != nil {
		return err
	}

	tasks := tfServer.getTasks()
	fmt.Printf("Tasks exposed as MCP tools (%d):\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("  %s\n", t)
	}
	return nil
}

func runMCPConfig(cmd *cobra.Command, args []string) error {
	workdir, _ := os.Getwd()

	fmt.Println("========================================")
	fmt.Println("xplat MCP Server Configuration")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Add to your AI IDE settings (Claude Desktop, Cursor, etc.):")
	fmt.Println()
	fmt.Printf(`{
  "mcpServers": {
    "xplat": {
      "command": "xplat",
      "args": ["mcp", "serve"],
      "cwd": "%s"
    }
  }
}
`, workdir)
	fmt.Println()
	fmt.Println("This exposes ALL your Taskfile tasks as MCP tools!")
	fmt.Println()
	return nil
}
