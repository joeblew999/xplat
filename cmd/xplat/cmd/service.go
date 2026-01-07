package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/service"
	"github.com/spf13/cobra"
)

var (
	serviceWorkDir string
	serviceName    string
	serviceWithUI  bool
	serviceUIPort  string
)

// ServiceCmd is the parent command for service operations.
var ServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage xplat as a system service",
	Long: `Install and manage xplat as a system service.

On macOS, this installs as a LaunchAgent (user service).
On Linux, this installs as a systemd user service.
On Windows, this installs as a Windows service.

The service runs 'xplat dev' (process-compose) in the background,
keeping your development processes running automatically.

Examples:
  xplat service install     # Install service for current directory
  xplat service start       # Start the service
  xplat service status      # Check service status
  xplat service stop        # Stop the service
  xplat service uninstall   # Remove the service`,
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install xplat as a system service",
	RunE:  runServiceInstall,
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the xplat service",
	RunE:  runServiceUninstall,
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

func init() {
	ServiceCmd.PersistentFlags().StringVarP(&serviceWorkDir, "dir", "d", "", "Working directory (default: current directory)")
	ServiceCmd.PersistentFlags().StringVarP(&serviceName, "name", "n", "", "Service name (default: xplat-<dirname>)")

	// UI flags for start command
	serviceStartCmd.Flags().BoolVar(&serviceWithUI, "with-ui", false, "Start Task UI alongside the service")
	serviceStartCmd.Flags().StringVar(&serviceUIPort, "ui-port", "3000", "Port for Task UI (requires --with-ui)")

	ServiceCmd.AddCommand(serviceInstallCmd)
	ServiceCmd.AddCommand(serviceUninstallCmd)
	ServiceCmd.AddCommand(serviceStartCmd)
	ServiceCmd.AddCommand(serviceStopCmd)
	ServiceCmd.AddCommand(serviceRestartCmd)
	ServiceCmd.AddCommand(serviceStatusCmd)
	ServiceCmd.AddCommand(serviceRunCmd)
}

func getServiceConfig() service.Config {
	workDir := serviceWorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	cfg := service.ConfigForProject(workDir)
	cfg.Version = version // Pass version for auto-update checking
	if serviceName != "" {
		cfg.Name = serviceName
		cfg.DisplayName = fmt.Sprintf("xplat: %s", serviceName)
	}

	// UI config
	cfg.WithUI = serviceWithUI
	cfg.UIPort = serviceUIPort

	return cfg
}

func runServiceInstall(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Install(); err != nil {
		return err
	}

	fmt.Printf("Service '%s' installed\n", cfg.Name)
	fmt.Printf("  Platform: %s\n", mgr.Platform())
	fmt.Printf("  Working directory: %s\n", cfg.WorkDir)
	fmt.Println()
	fmt.Println("To start the service, run: xplat service start")
	return nil
}

func runServiceUninstall(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	// Try to stop first (ignore errors)
	_ = mgr.Stop()

	if err := mgr.Uninstall(); err != nil {
		return err
	}

	fmt.Printf("Service '%s' uninstalled\n", cfg.Name)
	return nil
}

func runServiceStart(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Start(); err != nil {
		return err
	}

	fmt.Printf("Service '%s' started\n", cfg.Name)
	if cfg.WithUI {
		fmt.Printf("Task UI: http://localhost:%s\n", cfg.UIPort)
	}
	return nil
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Stop(); err != nil {
		return err
	}

	fmt.Printf("Service '%s' stopped\n", cfg.Name)
	return nil
}

func runServiceRestart(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	if err := mgr.Restart(); err != nil {
		return err
	}

	fmt.Printf("Service '%s' restarted\n", cfg.Name)
	return nil
}

func runServiceStatus(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	status, err := mgr.Status()
	if err != nil {
		fmt.Printf("Service '%s': %s (error: %v)\n", cfg.Name, status, err)
		return nil
	}

	fmt.Printf("Service '%s': %s\n", cfg.Name, status)
	fmt.Printf("  Platform: %s\n", mgr.Platform())
	fmt.Printf("  Working directory: %s\n", cfg.WorkDir)
	return nil
}

func runServiceRun(cmd *cobra.Command, args []string) error {
	cfg := getServiceConfig()
	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}

	// This blocks and runs the service
	return mgr.Run()
}
