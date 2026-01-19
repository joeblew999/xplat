// xplat - Cross-platform utilities for Taskfile
//
// A single binary that provides consistent behavior across
// macOS, Linux, and Windows for common shell operations.
package main

import (
	"os"

	// Bootstrap MUST be imported first to set log level before process-compose initializes
	_ "github.com/joeblew999/xplat/internal/bootstrap"

	"github.com/joeblew999/xplat/cmd/xplat/cmd"
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "xplat",
		Short: "One binary to bootstrap and run any plat-* project",
		Long: `xplat is a single binary that bootstraps and runs plat-* projects.

WHY USE THIS?
  Instead of installing Task, process-compose, and various CLIs separately,
  xplat embeds them all. One binary, works on macOS/Linux/Windows.

TYPICAL WORKFLOW:
  1. xplat manifest bootstrap   # Create standard project files
  2. xplat gen all              # Generate CI, .gitignore, etc from xplat.yaml
  3. xplat task build           # Build your project (embedded Task)
  4. xplat process              # Run services (embedded process-compose)
  5. xplat pkg install <name>   # Add packages from registry

KEY COMMANDS:
  task      - Run Taskfile tasks (embedded Task runner)
  process   - Run services (embedded process-compose)
  gen       - Generate files from YOUR local xplat.yaml
  pkg       - Install packages from REMOTE registry
  manifest  - Inspect/validate/bootstrap manifests
  os        - Cross-platform utilities (rm, cp, mv, glob, etc.)`,
	}

	// Pass version to the version command
	cmd.SetVersion(Version)

	// P0 (core)
	rootCmd.AddCommand(cmd.VersionCmd)
	rootCmd.AddCommand(cmd.UpdateCmd)
	rootCmd.AddCommand(cmd.RunCmd)

	// P1 (OS utilities - grouped under 'os' subcommand)
	// cat, cp, env, envsubst, extract, fetch, git, glob, jq, mkdir, mv, rm, touch, version-file, which
	rootCmd.AddCommand(cmd.OsCmd)

	// P2 (binary management)
	rootCmd.AddCommand(cmd.BinaryCmd)

	// P3 (release orchestration)
	rootCmd.AddCommand(cmd.ReleaseCmd)

	// P4 (embedded Task runner - includes fmt, lint, test, archetypes, detect, explain subcommands)
	rootCmd.AddCommand(cmd.TaskCmd)

	// P5 (Package management)
	rootCmd.AddCommand(cmd.PkgCmd)

	// P6 (Process orchestration)
	rootCmd.AddCommand(cmd.ProcessCmd)

	// P8 (Generation from xplat.yaml manifest)
	rootCmd.AddCommand(cmd.GenCmd)

	// P9 (Manifest management)
	rootCmd.AddCommand(cmd.ManifestCmd)

	// P10 (Service management)
	rootCmd.AddCommand(cmd.ServiceCmd)

	// P11 (Sync operations - run as service or CLI)
	rootCmd.AddCommand(cmd.SyncGHCmd)
	rootCmd.AddCommand(cmd.SyncCFCmd)

	// P12 (MCP - Model Context Protocol server for AI IDEs)
	rootCmd.AddCommand(cmd.MCPCmd)

	// P13 (Task UI - Web interface for running tasks)
	rootCmd.AddCommand(cmd.UICmd)

	// P14 (Internal commands for xplat developers - gen, dev, docs)
	// Make DocsCmd a subcommand of InternalCmd
	cmd.InternalCmd.AddCommand(cmd.DocsCmd)
	rootCmd.AddCommand(cmd.InternalCmd)

	// P15 (Setup wizard for external service configuration)
	rootCmd.AddCommand(cmd.SetupCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
