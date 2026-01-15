package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/lockfile"
	"github.com/joeblew999/xplat/internal/manifest"
	"github.com/joeblew999/xplat/internal/taskfile"
)

var (
	genDir     string
	genOutput  string
	genRepoURL string
	genForce   bool
)

// GenCmd is the parent command for all generation from xplat.yaml.
var GenCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate project files from YOUR local xplat.yaml",
	Long: `Generate project files from your local xplat.yaml manifest.

Use this to generate standard project files based on YOUR project's manifest.
This reads xplat.yaml in the current directory and creates files like
CI workflows, .gitignore, .env.example, etc.

Compare with:
  - 'xplat pkg' installs packages from the REMOTE registry
  - 'xplat manifest' inspects/validates manifests without generating

Examples:
  xplat gen workflow     # Generate .github/workflows/ci.yml
  xplat gen gitignore    # Generate .gitignore
  xplat gen env          # Generate .env.example
  xplat gen taskfile     # Generate Taskfile with remote includes
  xplat gen process      # Generate process-compose.yaml
  xplat gen all          # Generate all of the above`,
}

var genWorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Generate .github/workflows/ci.yml",
	Long: `Generate a unified GitHub Actions CI workflow.

Creates a minimal workflow that:
- Sets up the project language (auto-detected: go, rust, bun)
- Installs xplat
- Runs: xplat task build, test, lint

The same commands work locally and in CI.`,
	RunE: runGenWorkflow,
}

var genGitignoreCmd = &cobra.Command{
	Use:   "gitignore",
	Short: "Generate .gitignore",
	Long: `Generate a .gitignore file based on xplat.yaml manifest.

Includes:
- Base patterns (build artifacts, IDE files, OS files)
- Binary name from manifest
- Custom patterns from manifest gitignore.patterns`,
	RunE: runGenGitignore,
}

var genEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Generate .env.example",
	Long:  `Generate a .env.example file from manifest environment variables.`,
	RunE:  runGenEnv,
}

var genTaskfileCmd = &cobra.Command{
	Use:   "taskfile",
	Short: "Generate Taskfile.generated.yml with remote includes from installed packages",
	Long: `Generate a Taskfile with remote includes from installed xplat packages.

This enables COMPOSABILITY - reusing tasks from other xplat packages.

The generated file includes remote taskfiles from packages you've installed
via 'xplat pkg install'. Each installed package that exposes a taskfile
becomes an include in your project.

Example workflow:
  xplat pkg install plat-nats     # Install a package
  xplat gen taskfile              # Generate includes
  task nats:run                   # Use tasks from the installed package

Requires packages to be installed first with 'xplat pkg install'.`,
	RunE: runGenTaskfile,
}

var genProcessCmd = &cobra.Command{
	Use:   "process",
	Short: "Generate pc.generated.yaml with processes from installed packages",
	Long: `Generate a process-compose file with processes from installed xplat packages.

This enables COMPOSABILITY - running processes from other xplat packages.

The generated file includes processes from packages you've installed
via 'xplat pkg install'. Each installed package that exposes a process
configuration becomes a process in your compose file.

Example workflow:
  xplat pkg install plat-nats     # Install a package
  xplat gen process               # Generate process definitions
  process-compose up              # Run all processes including installed packages

Requires packages to be installed first with 'xplat pkg install'.`,
	RunE: runGenProcess,
}

var genAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Generate all files from manifest",
	RunE:  runGenAll,
}

func init() {
	GenCmd.PersistentFlags().StringVarP(&genDir, "dir", "d", ".", "Directory containing xplat.yaml")
	GenCmd.PersistentFlags().StringVarP(&genOutput, "output", "o", ".", "Output directory")
	GenCmd.PersistentFlags().StringVar(&genRepoURL, "repo-url", "https://github.com/joeblew999", "Base URL for GitHub repos")
	GenCmd.PersistentFlags().BoolVarP(&genForce, "force", "f", false, "Overwrite existing files")

	GenCmd.AddCommand(genWorkflowCmd)
	GenCmd.AddCommand(genGitignoreCmd)
	GenCmd.AddCommand(genEnvCmd)
	GenCmd.AddCommand(genTaskfileCmd)
	GenCmd.AddCommand(genProcessCmd)
	GenCmd.AddCommand(genAllCmd)
}

func loadManifestForGen() (*manifest.Manifest, error) {
	loader := manifest.NewLoader()
	m, err := loader.LoadDir(genDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load xplat.yaml: %w", err)
	}
	return m, nil
}

func runGenWorkflow(cmd *cobra.Command, args []string) error {
	baseDir := genOutput
	if baseDir == "" {
		baseDir = "."
	}

	gen := manifest.NewGenerator(nil)
	if err := gen.GenerateWorkflowDir(baseDir); err != nil {
		return fmt.Errorf("failed to generate workflow: %w", err)
	}

	fmt.Printf("Generated %s/.github/workflows/ci.yml\n", baseDir)
	return nil
}

func runGenGitignore(cmd *cobra.Command, args []string) error {
	m, err := loadManifestForGen()
	if err != nil {
		return err
	}

	gitignorePath := filepath.Join(genOutput, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil && !genForce {
		return fmt.Errorf(".gitignore already exists, use --force to overwrite")
	}

	binaryName := m.Name
	if m.Binary != nil && m.Binary.Name != "" {
		binaryName = m.Binary.Name
	}

	opts := taskfile.GitignoreOptions{BinaryName: binaryName}
	if m.HasGitignore() {
		opts.Patterns = m.Gitignore.Patterns
	}

	if err := taskfile.GenerateGitignoreWithOptions(gitignorePath, opts); err != nil {
		return err
	}

	fmt.Printf("Generated %s\n", gitignorePath)
	return nil
}

func runGenEnv(cmd *cobra.Command, args []string) error {
	m, err := loadManifestForGen()
	if err != nil {
		return err
	}

	gen := manifest.NewGenerator([]*manifest.Manifest{m})
	outputPath := filepath.Join(genOutput, ".env.example")

	if err := gen.GenerateEnvExample(outputPath); err != nil {
		return err
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func runGenTaskfile(cmd *cobra.Command, args []string) error {
	// Load lockfile to get installed packages (optional - service tasks are always generated)
	var pkgs []lockfile.Package
	lf, err := lockfile.Load(genDir)
	if err == nil {
		pkgs = lf.PackagesWithTaskfile()
	}

	outputPath := filepath.Join(genOutput, "Taskfile.generated.yml")
	if err := generateTaskfileFromLockfile(pkgs, outputPath, genRepoURL); err != nil {
		return err
	}

	if len(pkgs) > 0 {
		fmt.Printf("Generated %s with %d package include(s)\n", outputPath, len(pkgs))
	} else {
		fmt.Printf("Generated %s with service tasks\n", outputPath)
	}
	return nil
}

func runGenProcess(cmd *cobra.Command, args []string) error {
	// Load lockfile to get installed packages
	lf, err := lockfile.Load(genDir)
	if err != nil {
		return fmt.Errorf("failed to load lockfile: %w", err)
	}

	// Get packages with process configuration
	pkgs := lf.PackagesWithProcess()
	if len(pkgs) == 0 {
		fmt.Println("No installed packages with process configuration found.")
		fmt.Println("Install packages first with: xplat pkg install <package>")
		return nil
	}

	outputPath := filepath.Join(genOutput, config.ProcessComposeGeneratedFile)
	if err := generateProcessFromLockfile(pkgs, outputPath); err != nil {
		return err
	}

	fmt.Printf("Generated %s with %d process(es)\n", outputPath, len(pkgs))
	return nil
}

func runGenAll(cmd *cobra.Command, args []string) error {
	m, err := loadManifestForGen()
	if err != nil {
		return err
	}

	gen := manifest.NewGenerator([]*manifest.Manifest{m})
	baseDir := genOutput

	// Generate workflow
	if err := gen.GenerateWorkflowDir(baseDir); err != nil {
		return fmt.Errorf("failed to generate workflow: %w", err)
	}
	fmt.Printf("Generated %s/.github/workflows/ci.yml\n", baseDir)

	// Generate .gitignore
	binaryName := m.Name
	if m.Binary != nil && m.Binary.Name != "" {
		binaryName = m.Binary.Name
	}
	opts := taskfile.GitignoreOptions{BinaryName: binaryName}
	if m.HasGitignore() {
		opts.Patterns = m.Gitignore.Patterns
	}
	gitignorePath := filepath.Join(baseDir, ".gitignore")
	if err := taskfile.GenerateGitignoreWithOptions(gitignorePath, opts); err != nil {
		return fmt.Errorf("failed to generate .gitignore: %w", err)
	}
	fmt.Printf("Generated %s\n", gitignorePath)

	// Generate .env.example
	envPath := filepath.Join(baseDir, ".env.example")
	if err := gen.GenerateEnvExample(envPath); err != nil {
		return fmt.Errorf("failed to generate .env.example: %w", err)
	}
	fmt.Printf("Generated %s\n", envPath)

	// Load lockfile for taskfile and process generation
	lf, err := lockfile.Load(genDir)
	if err != nil {
		return fmt.Errorf("failed to load lockfile: %w", err)
	}

	// Generate process-compose from lockfile (if packages have process config)
	processPkgs := lf.PackagesWithProcess()
	if len(processPkgs) > 0 {
		processPath := filepath.Join(baseDir, config.ProcessComposeGeneratedFile)
		if err := generateProcessFromLockfile(processPkgs, processPath); err != nil {
			return fmt.Errorf("failed to generate process-compose: %w", err)
		}
		fmt.Printf("Generated %s with %d process(es)\n", processPath, len(processPkgs))
	}

	// Generate Taskfile from lockfile (if packages have taskfile config)
	taskfilePkgs := lf.PackagesWithTaskfile()
	if len(taskfilePkgs) > 0 {
		taskfilePath := filepath.Join(baseDir, "Taskfile.generated.yml")
		if err := generateTaskfileFromLockfile(taskfilePkgs, taskfilePath, genRepoURL); err != nil {
			return fmt.Errorf("failed to generate Taskfile: %w", err)
		}
		fmt.Printf("Generated %s with %d package include(s)\n", taskfilePath, len(taskfilePkgs))
	}

	return nil
}

// generateTaskfileFromLockfile creates a Taskfile.generated.yml with remote includes
// from installed packages tracked in the lockfile.
func generateTaskfileFromLockfile(pkgs []lockfile.Package, outputPath, repoBaseURL string) error {
	var buf []byte
	buf = append(buf, []byte(`# Generated by: xplat gen taskfile
# Regenerate with: xplat gen taskfile
#
# This file includes remote taskfiles from installed xplat packages.
# Install packages with: xplat pkg install <package>

version: "3"

vars:
  # Use installed xplat binary (from GitHub releases or local build)
  XPLAT: xplat

`)...)

	// Add includes if there are packages
	if len(pkgs) > 0 {
		buf = append(buf, []byte("includes:\n")...)
		for _, pkg := range pkgs {
			if pkg.Taskfile == nil {
				continue
			}
			ns := pkg.Taskfile.Namespace
			if ns == "" {
				ns = pkg.Name
			}
			url := pkg.Taskfile.URL
			if url == "" {
				// Build URL from source if not provided
				url = fmt.Sprintf("%s/%s.git//%s", repoBaseURL, pkg.Name, pkg.Taskfile.Path)
			}
			buf = append(buf, []byte(fmt.Sprintf("  %s:\n    taskfile: %s\n", ns, url))...)
		}
		buf = append(buf, '\n')
	}

	buf = append(buf, []byte(`tasks:
  default:
    desc: List available tasks
    cmds:
      - task --list

  # ===========================================================================
  # Service Commands (front and center!)
  # ===========================================================================
  # Start xplat as a system service with all features enabled:
  #   - Process orchestration (process-compose)
  #   - Task UI web dashboard
  #   - MCP HTTP server for AI IDE integration

  up:
    desc: "Start xplat service with UI + MCP (recommended)"
    cmds:
      - "{{.XPLAT}} service start --with-ui --with-mcp"

  up:ui:
    desc: "Start xplat service with UI only"
    cmds:
      - "{{.XPLAT}} service start --with-ui"

  up:mcp:
    desc: "Start xplat service with MCP only"
    cmds:
      - "{{.XPLAT}} service start --with-mcp"

  up:minimal:
    desc: "Start xplat service (process-compose only)"
    cmds:
      - "{{.XPLAT}} service start"

  down:
    desc: "Stop xplat service"
    cmds:
      - "{{.XPLAT}} service stop"

  status:
    desc: "Show xplat service status"
    cmds:
      - "{{.XPLAT}} service status"

  restart:
    desc: "Restart xplat service"
    cmds:
      - "{{.XPLAT}} service restart"
`)...)

	return os.WriteFile(outputPath, buf, 0644)
}

// generateProcessFromLockfile creates a pc.generated.yaml with processes
// from installed packages tracked in the lockfile.
func generateProcessFromLockfile(pkgs []lockfile.Package, outputPath string) error {
	var buf []byte
	buf = append(buf, []byte(`# Generated by: xplat gen process
# Regenerate with: xplat gen process
#
# This file includes processes from installed xplat packages.
# Install packages with: xplat pkg install <package>

version: "0.5"

processes:
`)...)

	for _, pkg := range pkgs {
		if pkg.Process == nil {
			continue
		}
		procName := pkg.Process.Name
		if procName == "" {
			procName = pkg.Name
		}
		buf = append(buf, []byte(fmt.Sprintf("  %s:\n    command: %s\n    availability:\n      restart: on_failure\n", procName, pkg.Process.Command))...)
	}

	return os.WriteFile(outputPath, buf, 0644)
}
