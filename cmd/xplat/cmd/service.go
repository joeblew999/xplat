package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/projects"
	"github.com/joeblew999/xplat/internal/service"
)

var (
	serviceWithUI bool
	serviceUIPort string
)

// ServiceCmd is the parent command for service operations.
var ServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage xplat as a system service",
	Long: `Manage xplat as a system service that runs all registered projects.

One global xplat service runs all projects from the registry.
Use 'install' from each project directory to add it to the registry.

On macOS: LaunchAgent (user service)
On Linux: systemd user service
On Windows: Windows service

Examples:
  cd ~/project1 && xplat service install  # Add project to registry
  cd ~/project2 && xplat service install  # Add another project
  xplat service list                       # Show all registered projects
  xplat service start                      # Start THE service (runs all projects)
  xplat service status                     # Check service status
  xplat service stop                       # Stop the service`,
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Add current project to registry and install OS service",
	Long: `Add the current project to the xplat registry and install the OS service.

This is idempotent - safe to run multiple times.
Run this from each project directory you want managed by xplat.`,
	RunE: runServiceInstall,
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove current project from registry",
	Long: `Remove the current project from the xplat registry.

The OS service is only removed when no projects remain in the registry.`,
	RunE: runServiceUninstall,
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the xplat service",
	Long: `Start the xplat service.

Use --with-ui to also start the Task UI web interface alongside the service.

Examples:
  xplat service start              # Start service only
  xplat service start --with-ui    # Start service + Task UI on port 3000
  xplat service start --with-ui --ui-port 8080  # Task UI on port 8080`,
	RunE: runServiceStart,
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the xplat service",
	RunE:  runServiceStop,
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the xplat service",
	RunE:  runServiceRestart,
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check service status",
	RunE:  runServiceStatus,
}

var serviceRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run the service (called by service manager)",
	Hidden: true, // Internal use only
	RunE:   runServiceRun,
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered projects",
	Long: `List all projects registered in the local xplat registry.

The registry is stored at ~/.xplat/projects.yaml and tracks all projects
that have been added via 'xplat service install'.

Examples:
  xplat service list     # Show all registered projects`,
	RunE: runServiceList,
}

func init() {
	// UI flags for start command
	serviceStartCmd.Flags().BoolVar(&serviceWithUI, "with-ui", false, "Start Task UI alongside the service")
	serviceStartCmd.Flags().StringVar(&serviceUIPort, "ui-port", config.DefaultUIPort, "Port for Task UI (requires --with-ui)")

	ServiceCmd.AddCommand(serviceInstallCmd)
	ServiceCmd.AddCommand(serviceUninstallCmd)
	ServiceCmd.AddCommand(serviceStartCmd)
	ServiceCmd.AddCommand(serviceStopCmd)
	ServiceCmd.AddCommand(serviceRestartCmd)
	ServiceCmd.AddCommand(serviceStatusCmd)
	ServiceCmd.AddCommand(serviceRunCmd)
	ServiceCmd.AddCommand(serviceListCmd)
}

// getGlobalServiceConfig returns config for THE one global xplat service.
func getGlobalServiceConfig() service.Config {
	cfg := service.DefaultConfig()
	cfg.Version = version
	cfg.WithUI = serviceWithUI
	cfg.UIPort = serviceUIPort
	return cfg
}

func runServiceInstall(cmd *cobra.Command, args []string) error {
	// Add current project to registry
	workDir, _ := os.Getwd()

	reg, err := projects.Load()
	if err != nil {
		return fmt.Errorf("failed to load project registry: %w", err)
	}

	projectName, err := reg.Add(workDir)
	if err != nil {
		return fmt.Errorf("failed to add project to registry: %w", err)
	}

	if err := reg.Save(); err != nil {
		return fmt.Errorf("failed to save project registry: %w", err)
	}

	fmt.Printf("Project '%s' added to registry\n", projectName)

	// Install the global xplat OS service (may already exist)
	cfg := getGlobalServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Install(); err != nil {
		fmt.Printf("OS service 'xplat' already installed\n")
	} else {
		fmt.Printf("OS service 'xplat' installed (%s)\n", mgr.Platform())
	}

	fmt.Printf("Registry: %s\n", config.XplatProjects())
	fmt.Println()
	fmt.Println("To start: xplat service start")
	return nil
}

func runServiceUninstall(cmd *cobra.Command, args []string) error {
	// Remove current project from registry
	workDir, _ := os.Getwd()

	reg, err := projects.Load()
	if err != nil {
		return fmt.Errorf("failed to load project registry: %w", err)
	}

	projectName, err := reg.RemoveByPath(workDir)
	if err != nil {
		fmt.Printf("Note: %v\n", err)
	} else {
		if err := reg.Save(); err != nil {
			return fmt.Errorf("failed to save project registry: %w", err)
		}
		fmt.Printf("Project '%s' removed from registry\n", projectName)
	}

	// Only remove OS service if no projects remain
	if len(reg.Projects) == 0 {
		cfg := getGlobalServiceConfig()
		mgr, err := service.NewManager(cfg)
		if err != nil {
			return err
		}

		_ = mgr.Stop()
		if err := mgr.Uninstall(); err != nil {
			fmt.Printf("Note: OS service not found\n")
		} else {
			fmt.Printf("OS service 'xplat' uninstalled (no projects remaining)\n")
		}
	} else {
		fmt.Printf("%d project(s) remaining in registry\n", len(reg.Projects))
	}

	return nil
}

func runServiceStart(cmd *cobra.Command, args []string) error {
	cfg := getGlobalServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Start(); err != nil {
		return err
	}

	fmt.Println("Service 'xplat' started")
	if cfg.WithUI {
		fmt.Printf("Task UI: http://localhost:%s\n", cfg.UIPort)
	}
	return nil
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	cfg := getGlobalServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Stop(); err != nil {
		return err
	}

	fmt.Println("Service 'xplat' stopped")
	return nil
}

func runServiceRestart(cmd *cobra.Command, args []string) error {
	cfg := getGlobalServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Restart(); err != nil {
		return err
	}

	fmt.Println("Service 'xplat' restarted")
	return nil
}

func runServiceStatus(cmd *cobra.Command, args []string) error {
	cfg := getGlobalServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	status, err := mgr.Status()
	if err != nil {
		fmt.Printf("Service 'xplat': %s (error: %v)\n", status, err)
		return nil
	}

	fmt.Printf("Service 'xplat': %s\n", status)
	fmt.Printf("  Platform: %s\n", mgr.Platform())

	// Also show registered projects
	reg, _ := projects.Load()
	fmt.Printf("  Projects: %d registered\n", len(reg.Projects))

	return nil
}

func runServiceRun(cmd *cobra.Command, args []string) error {
	cfg := getGlobalServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	// This blocks and runs the service
	return mgr.Run()
}

func runServiceList(cmd *cobra.Command, args []string) error {
	reg, err := projects.Load()
	if err != nil {
		return fmt.Errorf("failed to load project registry: %w", err)
	}

	if len(reg.Projects) == 0 {
		fmt.Println("No projects registered.")
		fmt.Printf("Registry: %s\n", config.XplatProjects())
		fmt.Println()
		fmt.Println("To add a project, run 'xplat service install' in the project directory.")
		return nil
	}

	// Sort project names for consistent output
	names := make([]string, 0, len(reg.Projects))
	for name := range reg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("Registered projects (%d):\n", len(reg.Projects))
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tENABLED\tPATH\tCONFIG")

	searchOrder := config.ProcessComposeSearchOrder()
	for _, name := range names {
		proj := reg.Projects[name]
		enabled := "yes"
		if !proj.Enabled {
			enabled = "no"
		}

		// Find config file
		configFile := "-"
		for _, cfgName := range searchOrder {
			cfgPath := proj.Path + "/" + cfgName
			if _, err := os.Stat(cfgPath); err == nil {
				configFile = cfgName
				break
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, enabled, proj.Path, configFile)
	}
	w.Flush()

	fmt.Println()
	fmt.Printf("Registry: %s\n", config.XplatProjects())

	return nil
}
