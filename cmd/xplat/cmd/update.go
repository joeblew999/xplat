package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/joeblew999/xplat/internal/updater"
	"github.com/spf13/cobra"
)

var (
	updateCheck bool
	updateForce bool
)

// UpdateCmd handles xplat self-update
var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update xplat to the latest version",
	Long: `Update xplat to the latest version from GitHub releases.

Examples:
  xplat update           # Update to latest version
  xplat update --check   # Check for updates without installing
  xplat update --force   # Force reinstall even if up to date`,
	RunE: runUpdate,
}

func init() {
	UpdateCmd.Flags().BoolVar(&updateCheck, "check", false, "Check for updates without installing")
	UpdateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if already up to date")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get current version
	currentVersion := version
	if currentVersion == "" {
		currentVersion = "dev"
	}

	// Fetch latest release info
	release, err := updater.GetLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := updater.ParseVersion(release.TagName)

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Latest version:  %s\n", latestVersion)

	// Check if update needed
	if !updateForce && currentVersion == latestVersion {
		fmt.Println("Already up to date.")
		return nil
	}

	if updateCheck {
		if currentVersion != latestVersion {
			fmt.Println("\nUpdate available! Run 'xplat update' to install.")
		}
		return nil
	}

	// Find the right asset for this platform
	downloadURL, err := updater.FindAssetURL(release)
	if err != nil {
		return err
	}

	assetName := updater.GetAssetName()
	fmt.Printf("\nDownloading %s...\n", assetName)

	newVersion, err := updater.Update(ctx, currentVersion, updateForce)
	if err != nil {
		return err
	}

	fmt.Printf("Updated xplat to %s\n", newVersion)
	_ = downloadURL // URL is used internally by Update
	return nil
}
