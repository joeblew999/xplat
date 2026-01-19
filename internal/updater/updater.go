// Package updater provides xplat self-update functionality.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joeblew999/xplat/internal/config"
)

// Use config constants for updater settings.
// See config.XplatRepo, config.XplatReleasesAPI, config.XplatChecksumFile, config.XplatTagPrefix

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
	req, err := http.NewRequestWithContext(ctx, "GET", config.XplatReleasesAPI, nil)
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
	return strings.TrimPrefix(tagName, config.XplatTagPrefix)
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

// FindChecksumURL finds the checksum file URL in a release.
func FindChecksumURL(release *Release) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == config.XplatChecksumFile {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no %s found in release", config.XplatChecksumFile)
}

// FetchChecksums downloads and parses the checksums file.
func FetchChecksums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		return nil, fmt.Errorf("failed to fetch checksums: HTTP %d", resp.StatusCode)
	}

	checksums := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 {
			checksums[parts[1]] = parts[0] // filename -> checksum
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return checksums, nil
}

// NeedsUpdate returns true if the current version differs from latest.
func NeedsUpdate(currentVersion, latestVersion string) bool {
	// Never update dev builds automatically
	if currentVersion == "" || currentVersion == "dev" {
		return false
	}
	return currentVersion != latestVersion
}

// GetExpectedChecksum fetches the expected checksum for the current platform from a release.
// Returns empty string if checksums are unavailable (caller should decide whether to warn/fail).
func GetExpectedChecksum(ctx context.Context, release *Release) string {
	checksumURL, err := FindChecksumURL(release)
	if err != nil {
		return ""
	}
	checksums, err := FetchChecksums(ctx, checksumURL)
	if err != nil {
		return ""
	}
	return checksums[GetAssetName()]
}

// DownloadAndReplace downloads a new binary and replaces the current one.
// On Unix, this uses atomic rename which is safe even if the binary is running.
// On Windows, we rename the old binary first since you can't delete a running exe.
func DownloadAndReplace(ctx context.Context, downloadURL, targetPath, expectedChecksum string) error {
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

	// Download to temp file in the same directory as target (required for atomic rename)
	targetDir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(targetDir, ".xplat-update-*")
	if err != nil {
		// Fall back to system temp if target dir doesn't work
		tmpFile, err = os.CreateTemp("", "xplat-update-*")
		if err != nil {
			return err
		}
	}
	tmpPath := tmpFile.Name()

	// Download and compute checksum simultaneously
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	tmpFile.Close()

	// Verify checksum
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedChecksum != "" && actualChecksum != expectedChecksum {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Replace binary (platform-specific)
	if runtime.GOOS == "windows" {
		// Windows: can't delete running exe, so rename it first
		oldPath := targetPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(targetPath, oldPath); err != nil {
			os.Remove(tmpPath)
			return err
		}
		if err := os.Rename(tmpPath, targetPath); err != nil {
			os.Rename(oldPath, targetPath) // Restore on failure
			os.Remove(tmpPath)
			return err
		}
		os.Remove(oldPath)
	} else {
		// Unix: atomic rename is safe even with running binary
		// The old inode stays valid for running processes until they exit
		// First remove the old file (unlinks it, but running processes keep their fd)
		os.Remove(targetPath)
		// Then rename new file into place
		if err := os.Rename(tmpPath, targetPath); err != nil {
			// If rename fails (e.g., cross-device), fall back to copy
			if err := copyFile(tmpPath, targetPath); err != nil {
				os.Remove(tmpPath)
				return err
			}
			os.Remove(tmpPath)
		}
	}

	return nil
}

// CanonicalInstallPath returns the canonical install location: ~/.local/bin/xplat
func CanonicalInstallPath() (string, error) {
	return config.XplatCanonicalBin(), nil
}

// CleanStaleBinaries removes xplat from non-canonical locations.
func CleanStaleBinaries() {
	for _, loc := range config.XplatStaleLocations() {
		if _, err := os.Stat(loc); err == nil {
			if err := os.Remove(loc); err == nil {
				fmt.Printf("Removed stale xplat from %s\n", loc)
			}
		}
	}
}

// Update performs a self-update of the xplat binary.
// Always installs to ~/.local/bin/xplat regardless of where current binary is running from.
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

	// Fetch checksums (optional, but warn if not available)
	expectedChecksum := GetExpectedChecksum(ctx, release)
	if expectedChecksum == "" {
		fmt.Fprintf(os.Stderr, "Warning: %s not found, skipping verification\n", config.XplatChecksumFile)
	}

	// Always install to canonical location
	installPath, err := CanonicalInstallPath()
	if err != nil {
		return "", err
	}

	// Ensure directory exists
	installDir := filepath.Dir(installPath)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := DownloadAndReplace(ctx, downloadURL, installPath, expectedChecksum); err != nil {
		return "", err
	}

	// Clean up any stale binaries in other locations
	CleanStaleBinaries()

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
