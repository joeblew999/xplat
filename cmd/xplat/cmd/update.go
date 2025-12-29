package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	xplatRepo   = "joeblew999/xplat"
	releasesAPI = "https://api.github.com/repos/" + xplatRepo + "/releases/latest"
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

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Get current version
	currentVersion := version
	if currentVersion == "" {
		currentVersion = "dev"
	}

	// Fetch latest release info
	release, err := getLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	// Extract version from tag (xplat-v0.3.0 -> v0.3.0)
	latestVersion := strings.TrimPrefix(release.TagName, "xplat-")

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
	assetName := getAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (looking for %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	fmt.Printf("\nDownloading %s...\n", assetName)

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "xplat-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(downloadURL)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write update: %w", err)
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// On Windows, we can't replace a running executable directly
	// Move old binary to .old, then move new binary in place
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		os.Remove(oldPath) // Remove any existing .old file
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to backup old binary: %w", err)
		}
		if err := copyFile(tmpPath, execPath); err != nil {
			// Try to restore old binary
			os.Rename(oldPath, execPath)
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		os.Remove(oldPath)
	} else {
		// Unix: atomic rename
		if err := copyFile(tmpPath, execPath); err != nil {
			return fmt.Errorf("failed to install update: %w", err)
		}
	}

	fmt.Printf("Updated xplat to %s\n", latestVersion)
	return nil
}

func getLatestRelease() (*githubRelease, error) {
	resp, err := http.Get(releasesAPI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	return &release, nil
}

func getAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("xplat-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}
