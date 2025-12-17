package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"text/tabwriter"

	"github.com/joeblew999/xplat/internal/process"
	"github.com/joeblew999/xplat/internal/registry"
	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/spf13/cobra"
)

// PkgCmd is the parent command for package operations
var PkgCmd = &cobra.Command{
	Use:   "pkg",
	Short: "Package management from Ubuntu Software registry",
	Long: `Install and manage packages from the Ubuntu Software registry.

Each package can include:
- A CLI binary (installed to ~/.local/bin)
- A remote Taskfile include (added to your Taskfile.yml)

The registry is hosted at https://www.ubuntusoftware.net/pkg/registry.json

Examples:
  xplat pkg list                    # List available packages
  xplat pkg info mailerlite         # Show package details
  xplat pkg install mailerlite      # Install binary + add taskfile
  xplat pkg remove mailerlite       # Remove binary + taskfile include`,
}

var pkgInstallCmd = &cobra.Command{
	Use:   "install <package>",
	Short: "Install a package (binary + taskfile)",
	Long: `Install a package from the Ubuntu Software registry.

This will:
1. Download and install the binary (if package has one)
2. Add a remote taskfile include to your Taskfile.yml

The taskfile include uses Task's remote include feature:
  https://taskfile.dev/experiments/remote-taskfiles/

Requires TASK_X_REMOTE_TASKFILES=1 environment variable.`,
	Args: cobra.ExactArgs(1),
	RunE: runPkgInstall,
}

var pkgInfoCmd = &cobra.Command{
	Use:   "info <package>",
	Short: "Show package details",
	Args:  cobra.ExactArgs(1),
	RunE:  runPkgInfo,
}

var pkgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available packages",
	RunE:  runPkgList,
}

var pkgRemoveCmd = &cobra.Command{
	Use:   "remove <package>",
	Short: "Remove a package (binary + taskfile include)",
	Args:  cobra.ExactArgs(1),
	RunE:  runPkgRemove,
}

var (
	pkgTaskfile      string // Path to Taskfile.yml
	pkgForce         bool   // Force reinstall
	pkgNoTaskfile    bool   // Skip taskfile include
	pkgNoBinary      bool   // Skip binary install
	pkgWithProcess   bool   // Also add to process-compose.yaml
	pkgProcessConfig string // Path to process-compose.yaml
)

func init() {
	pkgInstallCmd.Flags().StringVar(&pkgTaskfile, "taskfile", "Taskfile.yml", "Path to Taskfile.yml")
	pkgInstallCmd.Flags().BoolVar(&pkgForce, "force", false, "Force reinstall even if already installed")
	pkgInstallCmd.Flags().BoolVar(&pkgNoTaskfile, "no-taskfile", false, "Skip adding taskfile include")
	pkgInstallCmd.Flags().BoolVar(&pkgNoBinary, "no-binary", false, "Skip binary installation")
	pkgInstallCmd.Flags().BoolVar(&pkgWithProcess, "with-process", false, "Also add process to process-compose.yaml")
	pkgInstallCmd.Flags().StringVar(&pkgProcessConfig, "process-config", "process-compose.yaml", "Path to process-compose.yaml")

	pkgRemoveCmd.Flags().StringVar(&pkgTaskfile, "taskfile", "Taskfile.yml", "Path to Taskfile.yml")
	pkgRemoveCmd.Flags().StringVar(&pkgProcessConfig, "process-config", "process-compose.yaml", "Path to process-compose.yaml")

	PkgCmd.AddCommand(pkgInstallCmd)
	PkgCmd.AddCommand(pkgInfoCmd)
	PkgCmd.AddCommand(pkgListCmd)
	PkgCmd.AddCommand(pkgRemoveCmd)
}

func runPkgInstall(cmd *cobra.Command, args []string) error {
	pkgName := args[0]

	// Fetch package info from registry
	client := registry.NewClient()
	pkg, err := client.GetPackage(pkgName)
	if err != nil {
		return fmt.Errorf("failed to find package: %w", err)
	}

	fmt.Printf("Installing %s %s...\n", pkg.Name, pkg.Version)

	var installedBinary, installedTaskfile, installedProcess bool

	// Install binary if package has one
	if pkg.HasBinary && !pkgNoBinary {
		if err := installBinary(pkg); err != nil {
			fmt.Printf("Warning: binary install failed: %v\n", err)
		} else {
			installedBinary = true
		}
	}

	// Add taskfile include if package has one
	if pkg.TaskfilePath != "" && !pkgNoTaskfile {
		if err := installTaskfile(pkg); err != nil {
			fmt.Printf("Warning: taskfile include failed: %v\n", err)
		} else {
			installedTaskfile = true
		}
	}

	// Add process to process-compose.yaml if requested and package has process config
	if pkgWithProcess && pkg.HasProcess() {
		if err := installProcess(pkg); err != nil {
			fmt.Printf("Warning: process config failed: %v\n", err)
		} else {
			installedProcess = true
		}
	}

	// Print summary
	fmt.Println()
	if installedBinary {
		fmt.Printf("✓ Installed %s binary to ~/.local/bin/\n", pkg.BinaryName)
	}
	if installedTaskfile {
		fmt.Printf("✓ Added remote include to %s\n", pkgTaskfile)
	}
	if installedProcess {
		fmt.Printf("✓ Added process to %s\n", pkgProcessConfig)
	}

	if !installedBinary && !installedTaskfile && !installedProcess {
		if !pkg.HasBinary && pkg.TaskfilePath == "" {
			fmt.Printf("Package %s is a library with no binary or taskfile.\n", pkg.Name)
			fmt.Printf("Import path: %s\n", pkg.ImportPath)
		}
		return nil
	}

	// Usage hints
	fmt.Println()
	if installedTaskfile {
		fmt.Printf("  Run: task %s:help\n", pkg.Name)
		fmt.Println()
		fmt.Println("  Note: Remote taskfiles require:")
		fmt.Println("    export TASK_X_REMOTE_TASKFILES=1")
	} else if installedBinary {
		fmt.Printf("  Run: %s --help\n", pkg.BinaryName)
	}

	// Show process hint if package has process but wasn't installed
	if pkg.HasProcess() && !installedProcess && !pkgWithProcess {
		fmt.Println()
		fmt.Println("  This package defines a server process:")
		fmt.Printf("    Port: %d, Health: %s\n", pkg.Process.Port, pkg.Process.HealthPath)
		fmt.Println()
		fmt.Printf("  To add it to process-compose.yaml:\n")
		fmt.Printf("    xplat pkg install %s --with-process\n", pkg.Name)
		fmt.Printf("    # or: xplat process-gen add %s\n", pkg.Name)
	}

	return nil
}

func runPkgInfo(cmd *cobra.Command, args []string) error {
	pkgName := args[0]

	client := registry.NewClient()
	pkg, err := client.GetPackage(pkgName)
	if err != nil {
		return err
	}

	fmt.Printf("Package: %s\n", pkg.Name)
	fmt.Printf("Version: %s\n", pkg.Version)
	fmt.Printf("Description: %s\n", pkg.Description)
	fmt.Printf("Import: %s\n", pkg.ImportPath)
	fmt.Printf("Repository: %s\n", pkg.RepoURL)
	fmt.Printf("License: %s\n", pkg.License)
	fmt.Printf("Author: %s\n", pkg.Author)

	if pkg.HasBinary {
		fmt.Printf("Binary: %s\n", pkg.BinaryName)
	}

	if pkg.TaskfilePath != "" {
		fmt.Println()
		fmt.Println("Taskfile include:")
		fmt.Printf("  %s:\n", pkg.Name)
		fmt.Printf("    taskfile: %s\n", pkg.TaskfileURL())
	}

	if pkg.HasProcess() {
		fmt.Println()
		fmt.Println("Process configuration:")
		fmt.Printf("  Command: %s\n", pkg.Process.Command)
		if pkg.Process.Port > 0 {
			fmt.Printf("  Port: %d\n", pkg.Process.Port)
		}
		if pkg.Process.HealthPath != "" {
			fmt.Printf("  Health: %s\n", pkg.Process.HealthPath)
		}
		if pkg.Process.Namespace != "" {
			fmt.Printf("  Namespace: %s\n", pkg.Process.Namespace)
		}
		if pkg.Process.Disabled {
			fmt.Printf("  Disabled: true (not started by default)\n")
		}
	}

	return nil
}

func runPkgList(cmd *cobra.Command, args []string) error {
	client := registry.NewClient()
	packages, err := client.ListPackages()
	if err != nil {
		return err
	}

	if len(packages) == 0 {
		fmt.Println("No packages found in registry.")
		return nil
	}

	// Sort by name
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tBINARY\tTASKFILE\tDESCRIPTION")
	fmt.Fprintln(w, "----\t-------\t------\t--------\t-----------")

	for _, pkg := range packages {
		hasBin := "-"
		if pkg.HasBinary {
			hasBin = "✓"
		}
		hasTask := "-"
		if pkg.TaskfilePath != "" {
			hasTask = "✓"
		}

		// Truncate description
		desc := pkg.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			pkg.Name, pkg.Version, hasBin, hasTask, desc)
	}

	return w.Flush()
}

func runPkgRemove(cmd *cobra.Command, args []string) error {
	pkgName := args[0]

	client := registry.NewClient()
	pkg, err := client.GetPackage(pkgName)
	if err != nil {
		return err
	}

	fmt.Printf("Removing %s...\n", pkg.Name)

	// Remove binary
	if pkg.HasBinary {
		if err := removeBinary(pkg); err != nil {
			fmt.Printf("Warning: failed to remove binary: %v\n", err)
		} else {
			fmt.Printf("✓ Removed %s binary\n", pkg.BinaryName)
		}
	}

	// Remove taskfile include
	if pkg.TaskfilePath != "" {
		if err := removeTaskfile(pkg); err != nil {
			fmt.Printf("Warning: failed to remove taskfile include: %v\n", err)
		} else {
			fmt.Printf("✓ Removed %s include from %s\n", pkg.Name, pkgTaskfile)
		}
	}

	return nil
}

// installBinary installs the package binary using xplat binary install
func installBinary(pkg *registry.Package) error {
	if pkg.BinaryName == "" {
		return fmt.Errorf("package has no binary name")
	}

	// Check if already installed (unless force)
	if !pkgForce {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		if path, err := exec.LookPath(pkg.BinaryName + ext); err == nil {
			fmt.Printf("Binary %s already installed at %s\n", pkg.BinaryName, path)
			return nil
		}
	}

	// Use xplat binary install
	// This reuses the existing binary install logic
	binaryArgs := []string{
		"binary", "install",
		pkg.BinaryName,
		pkg.Version,
		pkg.GitHubRepo(),
	}

	if pkgForce {
		binaryArgs = append(binaryArgs, "--force")
	}

	// Run as subprocess to reuse existing logic
	xplatPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find xplat: %w", err)
	}

	cmd := exec.Command(xplatPath, binaryArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// installTaskfile adds the remote taskfile include
func installTaskfile(pkg *registry.Package) error {
	if pkg.TaskfilePath == "" {
		return fmt.Errorf("package has no taskfile")
	}

	// Check if already included
	has, err := taskfile.HasInclude(pkgTaskfile, pkg.Name)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if has && !pkgForce {
		fmt.Printf("Taskfile include %s already exists in %s\n", pkg.Name, pkgTaskfile)
		return nil
	}

	include := taskfile.Include{
		Name:     pkg.Name,
		Taskfile: pkg.TaskfileURL(),
	}

	return taskfile.AddInclude(pkgTaskfile, include)
}

// removeBinary removes the installed binary
func removeBinary(pkg *registry.Package) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	var installDir string
	if runtime.GOOS == "windows" {
		installDir = filepath.Join(home, "bin")
	} else {
		installDir = filepath.Join(home, ".local", "bin")
	}

	binPath := filepath.Join(installDir, pkg.BinaryName+ext)

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at %s", binPath)
	}

	return os.Remove(binPath)
}

// removeTaskfile removes the taskfile include
func removeTaskfile(pkg *registry.Package) error {
	return taskfile.RemoveInclude(pkgTaskfile, pkg.Name)
}

// installProcess adds the package's process to process-compose.yaml
func installProcess(pkg *registry.Package) error {
	if !pkg.HasProcess() {
		return fmt.Errorf("package has no process config")
	}

	gen := process.NewGenerator(pkgProcessConfig)
	return gen.AddPackage(pkg.Name)
}

// removeProcess removes the package's process from process-compose.yaml
func removeProcess(pkg *registry.Package) error {
	gen := process.NewGenerator(pkgProcessConfig)
	return gen.RemovePackage(pkg.Name)
}

// Helper to truncate strings
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
