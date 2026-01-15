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

// ServiceCmd is the parent command for service operations.
var ServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage xplat as a system service",
	Long: `Manage xplat as a system service that runs all registered projects.

One global xplat service runs all projects from the registry.
Use 'install' from each project directory to add it to the registry.
Use 'config' to configure UI, MCP, and sync settings once.

On macOS: LaunchAgent (user service)
On Linux: systemd user service
On Windows: Windows service

Examples:
  cd ~/project1 && xplat service install  # Add project to registry
  xplat service config --ui --sync        # Enable UI and sync (configure once)
  xplat service start                      # Start THE service
  xplat service status                     # Check service status`,
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
	Long: `Start the xplat service using the configuration from ~/.xplat/service.yaml.

To change settings, use 'xplat service config' first.`,
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

// Config command flags
var (
	serviceConfigNoUI         bool
	serviceConfigUIPort       string
	serviceConfigNoMCP        bool
	serviceConfigMCPPort      string
	serviceConfigNoSync       bool
	serviceConfigSyncRepos    string
	serviceConfigSyncInterval string
	serviceConfigReset        bool
)

var serviceConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure service settings",
	Long: `Configure xplat service settings. Settings are stored in ~/.xplat/service.yaml.

By default, all features are ENABLED (UI, MCP, Sync).
Use --no-* flags to disable features. Without flags, shows current config.

Examples:
  xplat service config                # Show current config (defaults: all enabled)
  xplat service config --no-ui        # Disable Task UI
  xplat service config --no-sync      # Disable GitHub sync
  xplat service config --reset        # Reset to defaults (all enabled)`,
	RunE: runServiceConfig,
}

func init() {
	// Config command flags - use --no-* to disable (features ON by default)
	serviceConfigCmd.Flags().BoolVar(&serviceConfigNoUI, "no-ui", false, "Disable Task UI web interface")
	serviceConfigCmd.Flags().StringVar(&serviceConfigUIPort, "ui-port", "", "Port for Task UI (default: 3000)")
	serviceConfigCmd.Flags().BoolVar(&serviceConfigNoMCP, "no-mcp", false, "Disable MCP HTTP server")
	serviceConfigCmd.Flags().StringVar(&serviceConfigMCPPort, "mcp-port", "", "Port for MCP server (default: 8765)")
	serviceConfigCmd.Flags().BoolVar(&serviceConfigNoSync, "no-sync", false, "Disable GitHub sync poller")
	serviceConfigCmd.Flags().StringVar(&serviceConfigSyncRepos, "sync-repos", "", "Repos to poll (comma-separated, empty = auto-discover)")
	serviceConfigCmd.Flags().StringVar(&serviceConfigSyncInterval, "sync-interval", "", "Poll interval (default: 5m)")
	serviceConfigCmd.Flags().BoolVar(&serviceConfigReset, "reset", false, "Reset to defaults (all enabled)")

	ServiceCmd.AddCommand(serviceInstallCmd)
	ServiceCmd.AddCommand(serviceUninstallCmd)
	ServiceCmd.AddCommand(serviceConfigCmd)
	ServiceCmd.AddCommand(serviceStartCmd)
	ServiceCmd.AddCommand(serviceStopCmd)
	ServiceCmd.AddCommand(serviceRestartCmd)
	ServiceCmd.AddCommand(serviceStatusCmd)
	ServiceCmd.AddCommand(serviceRunCmd)
	ServiceCmd.AddCommand(serviceListCmd)
}

// getServiceConfig loads config from file and converts to service.Config.
func getServiceConfig() (service.Config, error) {
	svcCfg, err := config.LoadServiceConfig()
	if err != nil {
		return service.Config{}, fmt.Errorf("failed to load service config: %w", err)
	}
	svcCfg.ApplyDefaults()

	cfg := service.DefaultConfig()
	cfg.Version = version
	cfg.WithUI = svcCfg.UI
	cfg.UIPort = svcCfg.UIPort
	cfg.WithMCP = svcCfg.MCP
	cfg.MCPPort = svcCfg.MCPPort
	cfg.WithSync = svcCfg.Sync
	cfg.SyncRepos = svcCfg.SyncRepos
	cfg.SyncInterval = svcCfg.SyncInterval

	return cfg, nil
}

func runServiceConfig(cmd *cobra.Command, args []string) error {
	// Load existing config
	cfg, err := config.LoadServiceConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// If reset, restore defaults (all enabled)
	if serviceConfigReset {
		cfg = config.DefaultServiceConfig()
		if err := config.SaveServiceConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Service config reset to defaults (all features enabled)")
		return nil
	}

	// Check if any flags were set
	flagsSet := cmd.Flags().Changed("no-ui") || cmd.Flags().Changed("ui-port") ||
		cmd.Flags().Changed("no-mcp") || cmd.Flags().Changed("mcp-port") ||
		cmd.Flags().Changed("no-sync") || cmd.Flags().Changed("sync-repos") ||
		cmd.Flags().Changed("sync-interval")

	if flagsSet {
		// Update config with flags (--no-* disables features)
		if cmd.Flags().Changed("no-ui") {
			cfg.UI = !serviceConfigNoUI
		}
		if cmd.Flags().Changed("ui-port") {
			cfg.UIPort = serviceConfigUIPort
		}
		if cmd.Flags().Changed("no-mcp") {
			cfg.MCP = !serviceConfigNoMCP
		}
		if cmd.Flags().Changed("mcp-port") {
			cfg.MCPPort = serviceConfigMCPPort
		}
		if cmd.Flags().Changed("no-sync") {
			cfg.Sync = !serviceConfigNoSync
		}
		if cmd.Flags().Changed("sync-repos") {
			cfg.SyncRepos = serviceConfigSyncRepos
		}
		if cmd.Flags().Changed("sync-interval") {
			cfg.SyncInterval = serviceConfigSyncInterval
		}

		if err := config.SaveServiceConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Service config updated")
		fmt.Println()
	}

	// Show current config
	cfg.ApplyDefaults()
	fmt.Printf("Service config (%s):\n\n", config.XplatServiceConfig())

	fmt.Printf("  UI:            %v\n", cfg.UI)
	if cfg.UI {
		fmt.Printf("  UI Port:       %s\n", cfg.UIPort)
	}
	fmt.Printf("  MCP:           %v\n", cfg.MCP)
	if cfg.MCP {
		fmt.Printf("  MCP Port:      %s\n", cfg.MCPPort)
	}
	fmt.Printf("  Sync:          %v\n", cfg.Sync)
	if cfg.Sync {
		if cfg.SyncRepos != "" {
			fmt.Printf("  Sync Repos:    %s\n", cfg.SyncRepos)
		} else {
			fmt.Printf("  Sync Repos:    (auto-discover from Taskfile.yml)\n")
		}
		fmt.Printf("  Sync Interval: %s\n", cfg.SyncInterval)
	}

	return nil
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
	cfg, err := getServiceConfig()
	if err != nil {
		return err
	}

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
	fmt.Println("To configure: xplat service config --ui --sync")
	fmt.Println("To start:     xplat service start")
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
		cfg, err := getServiceConfig()
		if err != nil {
			return err
		}

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
	cfg, err := getServiceConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Start(); err != nil {
		return err
	}

	fmt.Println("Service 'xplat' started")
	if cfg.WithUI {
		fmt.Printf("  Task UI: http://localhost:%s\n", cfg.UIPort)
	}
	if cfg.WithMCP {
		fmt.Printf("  MCP:     http://localhost:%s/mcp\n", cfg.MCPPort)
	}
	if cfg.WithSync {
		if cfg.SyncRepos != "" {
			fmt.Printf("  Sync:    polling %s every %s\n", cfg.SyncRepos, cfg.SyncInterval)
		} else {
			fmt.Printf("  Sync:    auto-discover, polling every %s\n", cfg.SyncInterval)
		}
	}
	return nil
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	cfg, err := getServiceConfig()
	if err != nil {
		return err
	}

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
	cfg, err := getServiceConfig()
	if err != nil {
		return err
	}

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
	cfg, err := getServiceConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	status, statusErr := mgr.Status()
	if statusErr != nil {
		fmt.Printf("Service 'xplat': %s (error: %v)\n", status, statusErr)
	} else {
		fmt.Printf("Service 'xplat': %s\n", status)
	}

	fmt.Printf("  Platform: %s\n", mgr.Platform())

	// Show config summary
	if cfg.WithUI || cfg.WithMCP || cfg.WithSync {
		fmt.Printf("  Features:")
		if cfg.WithUI {
			fmt.Printf(" ui")
		}
		if cfg.WithMCP {
			fmt.Printf(" mcp")
		}
		if cfg.WithSync {
			fmt.Printf(" sync")
		}
		fmt.Println()
	}

	// Show registered projects
	reg, _ := projects.Load()
	fmt.Printf("  Projects: %d registered\n", len(reg.Projects))

	return nil
}

func runServiceRun(cmd *cobra.Command, args []string) error {
	cfg, err := getServiceConfig()
	if err != nil {
		return err
	}

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
