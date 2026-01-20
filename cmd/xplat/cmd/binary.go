package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/osutil"
)

// BinaryCmd is the parent command for binary operations
var BinaryCmd = &cobra.Command{
	Use:   "binary",
	Short: "Binary management commands",
	Long: `Commands for managing binary tool installation.

Provides a centralized way to install binaries that:
- First checks if the binary already exists (PATH or install dir)
- Builds from local source if Go is available
- Downloads from GitHub releases as fallback`,
}

// BinaryInstallCmd installs a binary (build or download)
var BinaryInstallCmd = &cobra.Command{
	Use:   "install <name> <version> <repo>",
	Short: "Install a binary (build from source or download)",
	Long: `Install a binary tool, using the best available strategy:

1. Check if binary exists in PATH - skip if found
2. Check if binary exists in install dir - skip if found
3. Build from source if toolchain available AND --source/--source-cargo provided
4. Download from GitHub release as fallback

Arguments:
  name      Binary name (e.g., "analytics" or "simple-shape-viewer")
  version   Version tag (e.g., "v0.1.0")
  repo      GitHub repo (e.g., "joeblew999/ubuntu-website")

Examples:
  # Go binary - build from source or download
  xplat binary install analytics v0.1.0 joeblew999/ubuntu-website --source ./cmd/analytics

  # Cargo binary - build from source or download
  xplat binary install simple-shape-viewer v0.1.0 joeblew999/plat-trunk --source-cargo ./.src/truck --example

  # Just download (no build tools needed)
  xplat binary install sitecheck v0.1.0 joeblew999/ubuntu-website

  # Force reinstall
  xplat binary install analytics v0.1.0 joeblew999/ubuntu-website --force`,
	Args: cobra.ExactArgs(3),
	RunE: runBinaryInstall,
}

var (
	binarySource      string
	binarySourceCargo string
	binaryExample     bool
	binaryDir         string
	binaryForce       bool
)

func init() {
	BinaryInstallCmd.Flags().StringVar(&binarySource, "source", "", "Local Go source path for building (e.g., ./cmd/analytics)")
	BinaryInstallCmd.Flags().StringVar(&binarySourceCargo, "source-cargo", "", "Local Cargo project path for building (e.g., ./.src/truck)")
	BinaryInstallCmd.Flags().BoolVar(&binaryExample, "example", false, "Build as cargo example (--example <name>) instead of binary")
	BinaryInstallCmd.Flags().StringVar(&binaryDir, "dir", "", "Install directory (default: ~/.local/bin or ~/bin on Windows)")
	BinaryInstallCmd.Flags().BoolVar(&binaryForce, "force", false, "Force reinstall even if binary exists")

	BinaryCmd.AddCommand(BinaryInstallCmd)
}

func runBinaryInstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	version := args[1]
	repo := args[2]

	// Default install directory
	installDir := binaryDir
	if installDir == "" {
		var err error
		installDir, err = osutil.UserBinDir()
		if err != nil {
			return fmt.Errorf("failed to get install directory: %w", err)
		}
	}

	// Binary extension for Windows
	ext := osutil.BinaryExtension()
	binPath := filepath.Join(installDir, name+ext)

	// Check if binary exists (unless --force)
	if !binaryForce {
		// Check PATH
		if path, err := exec.LookPath(name + ext); err == nil {
			fmt.Printf("OK: %s found at %s\n", name, path)
			return nil
		}
		// Check install directory
		if _, err := os.Stat(binPath); err == nil {
			fmt.Printf("OK: %s found at %s\n", name, binPath)
			return nil
		}
	}

	// Create install directory
	if err := os.MkdirAll(installDir, config.DefaultDirPerms); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	// Strategy 1: Build from Go source if Go available AND --source provided
	if binarySource != "" {
		if _, err := exec.LookPath("go"); err == nil {
			// Clean the path to normalize separators (handles both / and \ on all platforms)
			sourcePath := filepath.Clean(binarySource)

			// Only join with cwd if path is relative
			if !filepath.IsAbs(sourcePath) {
				if cwd, err := os.Getwd(); err == nil {
					sourcePath = filepath.Join(cwd, sourcePath)
				}
			}

			if info, err := os.Stat(sourcePath); err == nil && info.IsDir() {
				fmt.Printf("Building %s from Go source...\n", name)
				fmt.Printf("    Source: %s\n", sourcePath)
				buildCmd := exec.Command("go", "build", "-o", binPath, sourcePath)
				buildCmd.Stdout = os.Stdout
				buildCmd.Stderr = os.Stderr
				if err := buildCmd.Run(); err != nil {
					return fmt.Errorf("go build failed: %w", err)
				}
				fmt.Printf("OK: %s built from Go source\n", name)
				fmt.Printf("    Installed to: %s\n", binPath)
				return nil
			}
		}
	}

	// Strategy 2: Build from Cargo source if cargo available AND --source-cargo provided
	if binarySourceCargo != "" {
		if _, err := exec.LookPath("cargo"); err == nil {
			// Clean the path to normalize separators
			sourcePath := filepath.Clean(binarySourceCargo)

			// Only join with cwd if path is relative
			if !filepath.IsAbs(sourcePath) {
				if cwd, err := os.Getwd(); err == nil {
					sourcePath = filepath.Join(cwd, sourcePath)
				}
			}

			if info, err := os.Stat(sourcePath); err == nil && info.IsDir() {
				fmt.Printf("Building %s from Cargo source...\n", name)
				fmt.Printf("    Source: %s\n", sourcePath)

				// Build args: cargo build --release [--example name]
				buildArgs := []string{"build", "--release"}
				if binaryExample {
					buildArgs = append(buildArgs, "--example", name)
				} else {
					buildArgs = append(buildArgs, "--bin", name)
				}

				buildCmd := exec.Command("cargo", buildArgs...)
				buildCmd.Dir = sourcePath
				buildCmd.Stdout = os.Stdout
				buildCmd.Stderr = os.Stderr
				if err := buildCmd.Run(); err != nil {
					return fmt.Errorf("cargo build failed: %w", err)
				}

				// Cargo puts binaries in target/release/[examples/]<name>
				var builtBin string
				if binaryExample {
					builtBin = filepath.Join(sourcePath, "target", "release", "examples", name+ext)
				} else {
					builtBin = filepath.Join(sourcePath, "target", "release", name+ext)
				}

				// Copy to install dir
				if err := copyFile(builtBin, binPath); err != nil {
					return fmt.Errorf("failed to copy binary: %w", err)
				}

				fmt.Printf("OK: %s built from Cargo source\n", name)
				fmt.Printf("    Installed to: %s\n", binPath)
				return nil
			}
		}
	}

	// Strategy 3: Download from GitHub release
	fmt.Printf("Downloading %s %s from GitHub...\n", name, version)

	// Build download URL using the centralized naming function
	// Format: https://github.com/REPO/releases/download/VERSION/NAME-OS-ARCH[.exe]
	binName := binaryFilename(name, runtime.GOOS, runtime.GOARCH)

	// Handle "dev" version by using latest release
	downloadVersion := version
	var url string
	if version == "" || version == "dev" {
		// Use GitHub's special "latest" redirect URL
		url = fmt.Sprintf("https://github.com/%s/releases/latest/download/%s",
			repo, binName)
		downloadVersion = "latest"
	} else {
		url = fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
			repo, version, binName)
	}

	fmt.Printf("URL: %s\n", url)

	// Download binary
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d, release %s may not exist yet, install Go and use --source to build from source", resp.StatusCode, version)
	}

	// Create output file
	out, err := os.Create(binPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = out.Close() }()

	// Copy content
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		_ = os.Remove(binPath) // Clean up partial download
		return fmt.Errorf("download incomplete: %w", err)
	}

	// Make executable (no-op on Windows)
	if err := os.Chmod(binPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Printf("OK: %s %s installed (%d bytes)\n", name, downloadVersion, written)
	fmt.Printf("    Installed to: %s\n", binPath)

	return nil
}

// copyFile copies a file from src to dst, setting executable permissions.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return os.Chmod(dst, 0755)
}
