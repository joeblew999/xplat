// Package updater provides xplat self-update functionality.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	XplatRepo   = "joeblew999/xplat"
	ReleasesAPI = "https://api.github.com/repos/" + XplatRepo + "/releases/latest"
)

// Release represents a GitHub release.
type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetLatestRelease fetches the latest release info from GitHub.
func GetLatestRelease(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", ReleasesAPI, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	return &release, nil
}

// GetLatestVersion returns just the version string of the latest release.
func GetLatestVersion(ctx context.Context) (string, error) {
	release, err := GetLatestRelease(ctx)
	if err != nil {
		return "", err
	}
	return ParseVersion(release.TagName), nil
}

// ParseVersion extracts the version from a tag name (e.g., "xplat-v0.3.0" -> "v0.3.0").
func ParseVersion(tagName string) string {
	return strings.TrimPrefix(tagName, "xplat-")
}

// GetAssetName returns the expected asset name for the current platform.
func GetAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("xplat-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// FindAssetURL finds the download URL for the current platform in a release.
func FindAssetURL(release *Release) (string, error) {
	assetName := GetAssetName()
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no asset found for %s", assetName)
}

// NeedsUpdate returns true if the current version differs from latest.
func NeedsUpdate(currentVersion, latestVersion string) bool {
	// Never update dev builds automatically
	if currentVersion == "" || currentVersion == "dev" {
		return false
	}
	return currentVersion != latestVersion
}

// DownloadAndReplace downloads a new binary and replaces the current one.
func DownloadAndReplace(ctx context.Context, downloadURL, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "xplat-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return err
	}

	// Replace binary (platform-specific)
	if runtime.GOOS == "windows" {
		oldPath := targetPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(targetPath, oldPath); err != nil {
			return err
		}
		if err := copyFile(tmpPath, targetPath); err != nil {
			os.Rename(oldPath, targetPath) // Restore on failure
			return err
		}
		os.Remove(oldPath)
	} else {
		if err := copyFile(tmpPath, targetPath); err != nil {
			return err
		}
	}

	return nil
}

// Update performs a self-update of the xplat binary.
func Update(ctx context.Context, currentVersion string, force bool) (newVersion string, err error) {
	release, err := GetLatestRelease(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := ParseVersion(release.TagName)

	if !force && !NeedsUpdate(currentVersion, latestVersion) && currentVersion == latestVersion {
		return latestVersion, nil // Already up to date
	}

	downloadURL, err := FindAssetURL(release)
	if err != nil {
		return "", err
	}

	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	if err := DownloadAndReplace(ctx, downloadURL, execPath); err != nil {
		return "", err
	}

	return latestVersion, nil
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
