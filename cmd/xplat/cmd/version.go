package cmd

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/joeblew999/xplat/internal/updater"
	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersion sets the version string (called from main)
func SetVersion(v string) {
	version = v
}

// GetVersion returns the current version
func GetVersion() string {
	return version
}

var versionVerbose bool

// VersionCmd prints the version
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print xplat version",
	Long: `Print xplat version information.

Examples:
  xplat version         # Show version
  xplat version -v      # Show verbose info with update check`,
	Run: runVersion,
}

func init() {
	VersionCmd.Flags().BoolVarP(&versionVerbose, "verbose", "v", false, "Show verbose information including update check")
}

func runVersion(cmd *cobra.Command, args []string) {
	if !versionVerbose {
		fmt.Println(version)
		return
	}

	// Verbose output
	fmt.Printf("xplat %s\n", version)
	fmt.Printf("  Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Go:       %s\n", runtime.Version())

	// Check for updates
	fmt.Print("  Update:   ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latest, err := updater.GetLatestVersion(ctx)
	if err != nil {
		fmt.Printf("check failed (%v)\n", err)
		return
	}

	if version == "dev" {
		fmt.Printf("%s available (run: xplat update)\n", latest)
	} else if version == latest {
		fmt.Println("up to date")
	} else {
		fmt.Printf("%s available (run: xplat update)\n", latest)
	}
}
