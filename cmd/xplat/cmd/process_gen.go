package cmd

import (
	"fmt"
	"sort"

	"github.com/joeblew999/xplat/internal/process"
	"github.com/joeblew999/xplat/internal/registry"
	"github.com/spf13/cobra"
)

var processConfigPath string

// ProcessGenCmd provides process-compose.yaml generation from package registry.
var ProcessGenCmd = &cobra.Command{
	Use:   "process-gen",
	Short: "Generate process-compose.yaml from package registry",
	Long: `Generate and manage process-compose.yaml from the Ubuntu Software package registry.

This command allows you to:
- Generate a complete process-compose.yaml from all packages with process configs
- Add individual package processes to your config
- Remove package processes from your config

The registry at www.ubuntusoftware.net/pkg/registry.json contains process
metadata for packages that define server processes.

Examples:
  xplat process-gen generate              # Generate from all registry packages
  xplat process-gen add mailerlite        # Add mailerlite process
  xplat process-gen remove mailerlite     # Remove mailerlite process
  xplat process-gen list                  # List packages with process configs`,
}

var processGenGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate process-compose.yaml from all registry packages",
	Long: `Generate a complete process-compose.yaml from all packages in the registry
that define process configurations.

This will overwrite the existing process-compose.yaml file.`,
	RunE: runProcessGenGenerate,
}

var processGenAddCmd = &cobra.Command{
	Use:   "add <package>",
	Short: "Add a package's process to process-compose.yaml",
	Long: `Add a single package's process configuration to the existing
process-compose.yaml file. Creates the file if it doesn't exist.`,
	Args: cobra.ExactArgs(1),
	RunE: runProcessGenAdd,
}

var processGenRemoveCmd = &cobra.Command{
	Use:   "remove <package>",
	Short: "Remove a package's process from process-compose.yaml",
	Args:  cobra.ExactArgs(1),
	RunE:  runProcessGenRemove,
}

var processGenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List packages with process configurations",
	Long:  `List all packages in the registry that define process configurations.`,
	RunE:  runProcessGenList,
}

func init() {
	ProcessGenCmd.PersistentFlags().StringVarP(&processConfigPath, "config", "f", "process-compose.yaml", "Path to process-compose.yaml")

	ProcessGenCmd.AddCommand(processGenGenerateCmd)
	ProcessGenCmd.AddCommand(processGenAddCmd)
	ProcessGenCmd.AddCommand(processGenRemoveCmd)
	ProcessGenCmd.AddCommand(processGenListCmd)
}

func runProcessGenGenerate(cmd *cobra.Command, args []string) error {
	gen := process.NewGenerator(processConfigPath)

	config, err := gen.GenerateFromRegistry()
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	if len(config.Processes) == 0 {
		fmt.Println("No packages with process configurations found in registry.")
		return nil
	}

	if err := gen.Write(config); err != nil {
		return err
	}

	fmt.Printf("✓ Generated %s with %d processes:\n", gen.ConfigPath(), len(config.Processes))
	for name := range config.Processes {
		fmt.Printf("  - %s\n", name)
	}

	return nil
}

func runProcessGenAdd(cmd *cobra.Command, args []string) error {
	pkgName := args[0]
	gen := process.NewGenerator(processConfigPath)

	if err := gen.AddPackage(pkgName); err != nil {
		return err
	}

	fmt.Printf("✓ Added %s to %s\n", pkgName, gen.ConfigPath())
	return nil
}

func runProcessGenRemove(cmd *cobra.Command, args []string) error {
	pkgName := args[0]
	gen := process.NewGenerator(processConfigPath)

	if err := gen.RemovePackage(pkgName); err != nil {
		return err
	}

	fmt.Printf("✓ Removed %s from %s\n", pkgName, gen.ConfigPath())
	return nil
}

func runProcessGenList(cmd *cobra.Command, args []string) error {
	client := registry.NewClient()
	packages, err := client.ListPackages()
	if err != nil {
		return fmt.Errorf("failed to fetch registry: %w", err)
	}

	var withProcess []registry.Package
	for _, pkg := range packages {
		if pkg.HasProcess() {
			withProcess = append(withProcess, pkg)
		}
	}

	if len(withProcess) == 0 {
		fmt.Println("No packages with process configurations found.")
		return nil
	}

	// Sort by name
	sort.Slice(withProcess, func(i, j int) bool {
		return withProcess[i].Name < withProcess[j].Name
	})

	fmt.Printf("Packages with process configurations (%d):\n\n", len(withProcess))
	for _, pkg := range withProcess {
		disabled := ""
		if pkg.Process.Disabled {
			disabled = " (disabled by default)"
		}
		fmt.Printf("  %s%s\n", pkg.Name, disabled)
		fmt.Printf("    Command: %s\n", pkg.Process.Command)
		if pkg.Process.Port > 0 {
			fmt.Printf("    Port: %d\n", pkg.Process.Port)
		}
		if pkg.Process.HealthPath != "" {
			fmt.Printf("    Health: %s\n", pkg.Process.HealthPath)
		}
		if pkg.Process.Namespace != "" {
			fmt.Printf("    Namespace: %s\n", pkg.Process.Namespace)
		}
		fmt.Println()
	}

	return nil
}
