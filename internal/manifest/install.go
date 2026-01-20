package manifest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// Installer installs binaries from manifests.
type Installer struct {
	installDir string
	force      bool
	verbose    bool
}

// NewInstaller creates a new installer.
func NewInstaller() *Installer {
	home, _ := os.UserHomeDir()
	installDir := filepath.Join(home, ".local", "bin")
	if runtime.GOOS == "windows" {
		installDir = filepath.Join(home, "bin")
	}

	return &Installer{
		installDir: installDir,
	}
}

// WithInstallDir sets a custom install directory.
func (i *Installer) WithInstallDir(dir string) *Installer {
	i.installDir = dir
	return i
}

// WithForce enables force reinstall.
func (i *Installer) WithForce(force bool) *Installer {
	i.force = force
	return i
}

// WithVerbose enables verbose output.
func (i *Installer) WithVerbose(verbose bool) *Installer {
	i.verbose = verbose
	return i
}

// Install installs a binary from a manifest.
func (i *Installer) Install(m *Manifest) error {
	if !m.HasBinary() {
		return fmt.Errorf("manifest %s has no binary defined", m.Name)
	}

	bin := m.Binary
	if bin.Source == nil {
		return fmt.Errorf("manifest %s has no binary source defined", m.Name)
	}

	// Check if already installed
	binPath := i.binaryPath(bin.Name)
	if !i.force {
		if _, err := os.Stat(binPath); err == nil {
			if i.verbose {
				fmt.Printf("Binary %s already installed at %s\n", bin.Name, binPath)
			}
			return nil
		}
	}

	// Ensure install directory exists
	if err := os.MkdirAll(i.installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	// Install with fallback chain: Go → GitHub → NPM → URL
	// Each source is tried if configured, falling back to next on failure

	// Try Go install if configured and Go is available
	if bin.Source.Go != "" {
		if _, err := exec.LookPath("go"); err == nil {
			if err := i.installGo(bin.Name, bin.Source.Go, m.Version); err == nil {
				return nil
			} else if i.verbose {
				fmt.Printf("  Go install failed: %v, trying fallback...\n", err)
			}
		} else if i.verbose {
			fmt.Printf("  Go not found, trying fallback...\n")
		}
	}

	// Fallback to GitHub releases
	if bin.Source.GitHub != nil {
		if err := i.installGitHub(bin.Name, bin.Source.GitHub, m.Version); err == nil {
			return nil
		} else if i.verbose {
			fmt.Printf("  GitHub download failed: %v, trying fallback...\n", err)
		}
	}

	// Fallback to NPM
	if bin.Source.NPM != "" {
		if err := i.installNPM(bin.Name, bin.Source.NPM, m.Version); err == nil {
			return nil
		} else if i.verbose {
			fmt.Printf("  NPM install failed: %v, trying fallback...\n", err)
		}
	}

	// Fallback to direct URL
	if bin.Source.URL != "" {
		return i.installURL(bin.Name, bin.Source.URL, m.Version)
	}

	return fmt.Errorf("all installation methods failed or no sources configured")
}

// binaryPath returns the full path for an installed binary.
func (i *Installer) binaryPath(name string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return filepath.Join(i.installDir, name+ext)
}

// installGo installs using go install.
func (i *Installer) installGo(name, importPath, version string) error {
	// Append version if specified
	path := importPath
	if version != "" && version != "dev" && !strings.HasSuffix(path, "@") {
		path = path + "@" + version
	} else if !strings.Contains(path, "@") {
		path = path + "@latest"
	}

	if i.verbose {
		fmt.Printf("Running: go install %s\n", path)
	}

	cmd := exec.Command("go", "install", path)
	cmd.Env = append(os.Environ(), "GOBIN="+i.installDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install failed: %w", err)
	}

	fmt.Printf("✓ Installed %s via go install\n", name)
	return nil
}

// installGitHub installs from GitHub releases.
func (i *Installer) installGitHub(name string, gh *GitHubSource, version string) error {
	if gh.Repo == "" {
		return fmt.Errorf("GitHub source missing repo")
	}

	// Build asset name with OS/ARCH substitution
	assetName := gh.Asset
	if assetName == "" {
		assetName = name + "-{{.OS}}-{{.ARCH}}"
	}

	// Template substitution
	tmpl, err := template.New("asset").Parse(assetName)
	if err != nil {
		return fmt.Errorf("invalid asset template: %w", err)
	}

	var buf bytes.Buffer
	data := map[string]string{
		"OS":   runtime.GOOS,
		"ARCH": runtime.GOARCH,
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("asset template failed: %w", err)
	}
	assetName = buf.String()

	// Build download URL
	ref := version
	if ref == "" || ref == "dev" {
		ref = "latest"
	}

	var downloadURL string
	if ref == "latest" {
		downloadURL = fmt.Sprintf("https://github.com/%s/releases/latest/download/%s",
			gh.Repo, assetName)
	} else {
		downloadURL = fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
			gh.Repo, ref, assetName)
	}

	if i.verbose {
		fmt.Printf("Downloading: %s\n", downloadURL)
	}

	// Use xplat fetch to download
	xplatPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find xplat: %w", err)
	}

	binPath := i.binaryPath(name)
	cmd := exec.Command(xplatPath, "fetch", downloadURL, "-o", binPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Make executable
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binPath, 0755); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	// macOS: Remove quarantine attribute to allow unsigned binary to run
	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", binPath).Run()
	}

	fmt.Printf("✓ Installed %s from GitHub release\n", name)
	return nil
}

// installNPM installs using npm/bun.
func (i *Installer) installNPM(name, pkg, version string) error {
	// Prefer bun if available
	npmCmd := "npm"
	if _, err := exec.LookPath("bun"); err == nil {
		npmCmd = "bun"
	}

	// Build package spec
	pkgSpec := pkg
	if version != "" && version != "dev" {
		pkgSpec = pkg + "@" + version
	}

	if i.verbose {
		fmt.Printf("Running: %s install -g %s\n", npmCmd, pkgSpec)
	}

	cmd := exec.Command(npmCmd, "install", "-g", pkgSpec)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s install failed: %w", npmCmd, err)
	}

	fmt.Printf("✓ Installed %s via %s\n", name, npmCmd)
	return nil
}

// installURL installs from a direct URL.
func (i *Installer) installURL(name, urlTemplate, version string) error {
	// Template substitution
	tmpl, err := template.New("url").Parse(urlTemplate)
	if err != nil {
		return fmt.Errorf("invalid URL template: %w", err)
	}

	var buf bytes.Buffer
	data := map[string]string{
		"OS":      runtime.GOOS,
		"ARCH":    runtime.GOARCH,
		"VERSION": version,
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("URL template failed: %w", err)
	}
	downloadURL := buf.String()

	if i.verbose {
		fmt.Printf("Downloading: %s\n", downloadURL)
	}

	// Use xplat fetch
	xplatPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find xplat: %w", err)
	}

	binPath := i.binaryPath(name)
	cmd := exec.Command(xplatPath, "fetch", downloadURL, "-o", binPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Make executable
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binPath, 0755); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	// macOS: Remove quarantine attribute to allow unsigned binary to run
	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", binPath).Run()
	}

	fmt.Printf("✓ Installed %s from URL\n", name)
	return nil
}

// Uninstall removes an installed binary.
func (i *Installer) Uninstall(m *Manifest) error {
	if !m.HasBinary() {
		return fmt.Errorf("manifest %s has no binary defined", m.Name)
	}

	binPath := i.binaryPath(m.Binary.Name)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not installed at %s", binPath)
	}

	if err := os.Remove(binPath); err != nil {
		return fmt.Errorf("failed to remove binary: %w", err)
	}

	fmt.Printf("✓ Removed %s\n", m.Binary.Name)
	return nil
}

// IsInstalled checks if a binary is installed.
func (i *Installer) IsInstalled(m *Manifest) bool {
	if !m.HasBinary() {
		return false
	}
	_, err := os.Stat(i.binaryPath(m.Binary.Name))
	return err == nil
}
