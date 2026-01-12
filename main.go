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
		Short: "Cross-platform utilities for Taskfile",
		Long: `xplat provides cross-platform shell utilities that work
identically on macOS, Linux, and Windows.

Designed to fill gaps in Task's built-in shell interpreter.`,
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
	rootCmd.AddCommand(cmd.ProcessGenCmd)

	// P7 (Documentation generation)
	rootCmd.AddCommand(cmd.DocsCmd)

	// P8 (Manifest management)
	rootCmd.AddCommand(cmd.ManifestCmd)

	// P9 (Service management)
	rootCmd.AddCommand(cmd.ServiceCmd)

	// P10 (Sync operations - run as service or CLI)
	rootCmd.AddCommand(cmd.SyncGHCmd)
	rootCmd.AddCommand(cmd.SyncCFCmd)

	// Note: Web UI is available via `xplat dev ui` (starts process-compose + UI together)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
