// xplat - Cross-platform utilities for Taskfile
//
// A single binary that provides consistent behavior across
// macOS, Linux, and Windows for common shell operations.
package main

import (
	"os"

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

	// Add subcommands - P0 (core)
	rootCmd.AddCommand(cmd.VersionCmd)
	rootCmd.AddCommand(cmd.WhichCmd)
	rootCmd.AddCommand(cmd.RunCmd)
	rootCmd.AddCommand(cmd.GlobCmd)

	// P1 (file operations)
	rootCmd.AddCommand(cmd.RmCmd)
	rootCmd.AddCommand(cmd.MkdirCmd)
	rootCmd.AddCommand(cmd.CpCmd)
	rootCmd.AddCommand(cmd.MvCmd)

	// P2 (utilities)
	rootCmd.AddCommand(cmd.CatCmd)
	rootCmd.AddCommand(cmd.TouchCmd)
	rootCmd.AddCommand(cmd.EnvCmd)
	rootCmd.AddCommand(cmd.JqCmd)
	rootCmd.AddCommand(cmd.VersionFileCmd)
	rootCmd.AddCommand(cmd.GitCmd)

	// P3 (binary management)
	rootCmd.AddCommand(cmd.BinaryCmd)

	// P4 (archive operations)
	rootCmd.AddCommand(cmd.ExtractCmd)
	rootCmd.AddCommand(cmd.FetchCmd)

	// P4 (release orchestration)
	rootCmd.AddCommand(cmd.ReleaseCmd)

	// P5 (embedded Task runner)
	rootCmd.AddCommand(cmd.TaskCmd)

	// P6 (Taskfile validation)
	rootCmd.AddCommand(cmd.FmtCmd)
	rootCmd.AddCommand(cmd.LintCmd)
	rootCmd.AddCommand(cmd.TaskfileCmd)

	// P7 (Taskfile testing)
	rootCmd.AddCommand(cmd.TestCmd)

	// P8 (Package management)
	rootCmd.AddCommand(cmd.PkgCmd)

	// P9 (Process orchestration)
	rootCmd.AddCommand(cmd.ProcessCmd)
	rootCmd.AddCommand(cmd.ProcessGenCmd)
	rootCmd.AddCommand(cmd.DevCmd)

	// P10 (Documentation generation)
	rootCmd.AddCommand(cmd.DocsCmd)

	// P11 (Manifest management)
	rootCmd.AddCommand(cmd.ManifestCmd)

	// P12 (Service management)
	rootCmd.AddCommand(cmd.ServiceCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
