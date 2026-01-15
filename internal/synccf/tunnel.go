package synccf

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/joeblew999/xplat/internal/config"
)

// TunnelConfig holds configuration for cloudflared tunnel
type TunnelConfig struct {
	Name            string // tunnel name (creates quick tunnel if empty)
	LocalPort       int    // local port to expose
	Hostname        string // custom hostname (optional, for named tunnels)
	Protocol        string // http or https (default: http)
	CloudflaredPath string // path to cloudflared binary (auto-detect if empty)
}

// Tunnel manages a cloudflared tunnel
type Tunnel struct {
	config  TunnelConfig
	cmd     *exec.Cmd
	url     string
	urlCh   chan string
	stopCh  chan struct{}
	mu      sync.Mutex
	running bool
}

// NewTunnel creates a new Cloudflare tunnel manager
func NewTunnel(cfg TunnelConfig) *Tunnel {
	if cfg.Protocol == "" {
		cfg.Protocol = "http"
	}
	if cfg.CloudflaredPath == "" {
		cfg.CloudflaredPath = "cloudflared"
	}

	return &Tunnel{
		config: cfg,
		urlCh:  make(chan string, 1),
		stopCh: make(chan struct{}),
	}
}

// Start starts the cloudflared tunnel
func (t *Tunnel) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("tunnel already running")
	}
	t.running = true
	t.mu.Unlock()

	// Build command args
	args := []string{"tunnel"}

	if t.config.Name == "" {
		// Quick tunnel (no account needed)
		args = append(args, "--url", fmt.Sprintf("%s://localhost:%d", t.config.Protocol, t.config.LocalPort))
	} else {
		// Named tunnel (requires auth)
		args = append(args, "run", t.config.Name)
	}

	t.cmd = exec.CommandContext(ctx, t.config.CloudflaredPath, args...)

	// Capture stderr for URL extraction (cloudflared outputs URL to stderr)
	stderr, err := t.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloudflared: %w", err)
	}

	log.Printf("sync-cf: tunnel starting (pid: %d)", t.cmd.Process.Pid)

	// Parse output for tunnel URL
	go t.parseOutput(bufio.NewReader(stderr), "stderr")
	go t.parseOutput(bufio.NewReader(stdout), "stdout")

	// Wait for URL or timeout
	select {
	case url := <-t.urlCh:
		t.url = url
		log.Printf("sync-cf: tunnel ready at %s", url)
	case <-time.After(30 * time.Second):
		t.Stop()
		return fmt.Errorf("timeout waiting for tunnel URL")
	case <-ctx.Done():
		t.Stop()
		return ctx.Err()
	}

	return nil
}

func (t *Tunnel) parseOutput(r *bufio.Reader, source string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("sync-cf [%s]: %s", source, line)

		if strings.Contains(line, "trycloudflare.com") || strings.Contains(line, ".cfargotunnel.com") {
			url := extractURL(line)
			if url != "" {
				select {
				case t.urlCh <- url:
				default:
				}
			}
		}
	}
}

func extractURL(line string) string {
	idx := strings.Index(line, "https://")
	if idx == -1 {
		return ""
	}

	url := line[idx:]
	if spaceIdx := strings.IndexAny(url, " \t\n\r"); spaceIdx != -1 {
		url = url[:spaceIdx]
	}

	return url
}

// Stop stops the tunnel
func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	if t.cmd != nil && t.cmd.Process != nil {
		log.Printf("sync-cf: stopping tunnel")
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}

	t.running = false
	close(t.stopCh)
}

// URL returns the tunnel's public URL
func (t *Tunnel) URL() string {
	return t.url
}

// IsRunning returns whether the tunnel is running
func (t *Tunnel) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// Wait waits for the tunnel to exit
func (t *Tunnel) Wait() error {
	if t.cmd == nil {
		return nil
	}
	return t.cmd.Wait()
}

// RunQuickTunnel starts a quick tunnel and returns the URL
func RunQuickTunnel(ctx context.Context, localPort int) (string, *Tunnel, error) {
	tunnel := NewTunnel(TunnelConfig{
		LocalPort: localPort,
	})

	if err := tunnel.Start(ctx); err != nil {
		return "", nil, err
	}

	return tunnel.URL(), tunnel, nil
}

// CloudflaredInfo contains version and path information
type CloudflaredInfo struct {
	Version string
	Path    string
}

// CheckCloudflared verifies cloudflared is installed and returns version info
func CheckCloudflared() error {
	info, err := GetCloudflaredInfo()
	if err != nil {
		return err
	}
	log.Printf("sync-cf: found cloudflared %s at %s", info.Version, info.Path)
	return nil
}

// GetCloudflaredInfo returns detailed cloudflared installation info
func GetCloudflaredInfo() (*CloudflaredInfo, error) {
	// Check standard PATH first
	path, err := exec.LookPath("cloudflared")
	if err != nil {
		// Check common install locations - xplat bin first
		binaryName := "cloudflared"
		if runtime.GOOS == "windows" {
			binaryName = "cloudflared.exe"
		}
		candidates := []string{
			filepath.Join(config.XplatBin(), binaryName),
			"/usr/local/bin/cloudflared",
			"/opt/homebrew/bin/cloudflared",
		}
		if runtime.GOOS == "windows" {
			candidates = append(candidates,
				filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "cloudflared", "cloudflared.exe"),
				filepath.Join(os.Getenv("ProgramFiles"), "cloudflared", "cloudflared.exe"),
			)
		}
		for _, c := range candidates {
			if fileExists(c) {
				path = c
				break
			}
		}
		if path == "" {
			return nil, fmt.Errorf("cloudflared not found (run: xplat sync-cf install)")
		}
	}

	cmd := exec.Command(path, "version")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cloudflared found but failed to get version: %w", err)
	}

	version := strings.TrimSpace(string(output))
	return &CloudflaredInfo{Version: version, Path: path}, nil
}

// GetLatestCloudflaredVersion fetches the latest release version from GitHub
func GetLatestCloudflaredVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/cloudflare/cloudflared/releases/latest")
	if err != nil {
		return "", fmt.Errorf("failed to check latest version: %w", err)
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	return release.TagName, nil
}

// InstallCloudflared downloads and installs cloudflared from GitHub releases
func InstallCloudflared() error {
	version, err := GetLatestCloudflaredVersion()
	if err != nil {
		return err
	}

	url, filename := getCloudflaredDownloadURL(version)
	if url == "" {
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	log.Printf("sync-cf: downloading cloudflared %s for %s/%s", version, runtime.GOOS, runtime.GOARCH)

	// Download to temp file
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download cloudflared: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download cloudflared: HTTP %d", resp.StatusCode)
	}

	// Determine install location - use xplat's global bin directory
	installDir := config.XplatBin()
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	binaryName := "cloudflared"
	if runtime.GOOS == "windows" {
		binaryName = "cloudflared.exe"
	}
	installPath := filepath.Join(installDir, binaryName)

	// Handle different download formats
	if strings.HasSuffix(filename, ".tgz") {
		// Extract from tarball (macOS)
		if err := extractCloudflaredTgz(resp.Body, installPath); err != nil {
			return err
		}
	} else {
		// Direct binary (Linux, Windows)
		out, err := os.OpenFile(installPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("failed to create binary: %w", err)
		}
		defer out.Close()

		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("failed to write binary: %w", err)
		}
	}

	log.Printf("sync-cf: installed cloudflared to %s", installPath)
	log.Printf("sync-cf: add to PATH: export PATH=\"%s:$PATH\"", installDir)

	return nil
}

func getCloudflaredDownloadURL(version string) (url, filename string) {
	base := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/download/%s", version)

	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			filename = "cloudflared-darwin-amd64.tgz"
		case "arm64":
			filename = "cloudflared-darwin-arm64.tgz"
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			filename = "cloudflared-linux-amd64"
		case "arm64":
			filename = "cloudflared-linux-arm64"
		case "arm":
			filename = "cloudflared-linux-arm"
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			filename = "cloudflared-windows-amd64.exe"
		case "386":
			filename = "cloudflared-windows-386.exe"
		}
	}

	if filename == "" {
		return "", ""
	}
	return base + "/" + filename, filename
}

func extractCloudflaredTgz(r io.Reader, destPath string) error {
	// Use xplat's extract functionality or shell out to tar
	// For simplicity, use tar command (available on macOS/Linux)
	tmpDir, err := os.MkdirTemp("", "cloudflared-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "cloudflared.tgz")
	out, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	out.Close()

	// Extract with tar
	cmd := exec.Command("tar", "-xzf", tmpFile, "-C", tmpDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract tarball: %w", err)
	}

	// Find and move the binary
	extracted := filepath.Join(tmpDir, "cloudflared")
	if !fileExists(extracted) {
		return fmt.Errorf("cloudflared binary not found in tarball")
	}

	// Copy to destination
	src, err := os.Open(extracted)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RunTunnel is the high-level API for running a cloudflared quick tunnel.
// It handles cloudflared installation, signal handling, and graceful shutdown.
// This is the main entry point for the CLI command.
func RunTunnel(ctx context.Context, port int) error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	tunnel := NewTunnel(TunnelConfig{
		LocalPort:       port,
		CloudflaredPath: cfPath,
	})

	log.Printf("Starting cloudflared quick tunnel for localhost:%d...", port)

	if err := tunnel.Start(ctx); err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	log.Printf("Tunnel URL: %s", tunnel.URL())
	log.Printf("   Webhook endpoint: %s/webhook", tunnel.URL())
	log.Printf("   CF webhook endpoint: %s/cf/webhook", tunnel.URL())
	log.Printf("")
	log.Printf("Press Ctrl+C to stop the tunnel")

	// Wait for context cancellation
	<-ctx.Done()
	tunnel.Stop()
	log.Printf("Tunnel stopped")

	return nil
}

// RunNamedTunnel runs a named cloudflared tunnel with a stable hostname.
// Named tunnels require prior setup via cloudflared tunnel login and cloudflared tunnel create.
// The hostname is tied to your Cloudflare domain (free with CF account).
func RunNamedTunnel(ctx context.Context, name string, port int) error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	tunnel := NewTunnel(TunnelConfig{
		Name:            name,
		LocalPort:       port,
		CloudflaredPath: cfPath,
	})

	log.Printf("Starting cloudflared named tunnel '%s' for localhost:%d...", name, port)

	if err := tunnel.Start(ctx); err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	// For named tunnels, URL is the configured hostname
	url := tunnel.URL()
	if url == "" {
		// Named tunnels don't output URL to stderr like quick tunnels
		// The URL is the hostname configured in the tunnel
		log.Printf("Named tunnel '%s' is running", name)
		log.Printf("   Use your configured hostname to access")
	} else {
		log.Printf("Tunnel URL: %s", url)
	}
	log.Printf("")
	log.Printf("Press Ctrl+C to stop the tunnel")

	// Wait for context cancellation
	<-ctx.Done()
	tunnel.Stop()
	log.Printf("Tunnel stopped")

	return nil
}

// getCloudflaredPath returns the path to cloudflared, installing it if needed.
func getCloudflaredPath() (string, error) {
	info, err := GetCloudflaredInfo()
	if err != nil {
		log.Printf("cloudflared not found, attempting install...")
		if err := InstallCloudflared(); err != nil {
			return "", fmt.Errorf("cloudflared not available: %w", err)
		}
		// Try again after install
		info, err = GetCloudflaredInfo()
		if err != nil {
			return "", fmt.Errorf("cloudflared still not found after install: %w", err)
		}
	}
	return info.Path, nil
}

// LoginCloudflared runs cloudflared tunnel login to authenticate with Cloudflare.
// This opens a browser for OAuth authentication.
func LoginCloudflared() error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	log.Printf("Opening browser for Cloudflare authentication...")
	log.Printf("This will create credentials at ~/.cloudflared/cert.pem")

	cmd := exec.Command(cfPath, "tunnel", "login")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// ListTunnels runs cloudflared tunnel list to show existing tunnels.
func ListTunnels() error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	cmd := exec.Command(cfPath, "tunnel", "list")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// CreateTunnel creates a new named tunnel.
func CreateTunnel(name string) error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	log.Printf("Creating tunnel '%s'...", name)

	cmd := exec.Command(cfPath, "tunnel", "create", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tunnel: %w", err)
	}

	log.Printf("Tunnel '%s' created successfully", name)
	log.Printf("")
	log.Printf("Next steps:")
	log.Printf("  1. Add DNS route: cloudflared tunnel route dns %s <hostname>", name)
	log.Printf("  2. Create config: ~/.cloudflared/config.yml")
	log.Printf("  3. Run tunnel:    xplat sync-cf tunnel --name=%s", name)

	return nil
}

// DeleteTunnel deletes a named tunnel.
func DeleteTunnel(name string) error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	log.Printf("Deleting tunnel '%s'...", name)

	cmd := exec.Command(cfPath, "tunnel", "delete", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RouteTunnelDNS creates a DNS CNAME record pointing to the tunnel.
func RouteTunnelDNS(tunnelName, hostname string) error {
	cfPath, err := getCloudflaredPath()
	if err != nil {
		return err
	}

	log.Printf("Creating DNS route: %s -> tunnel '%s'", hostname, tunnelName)

	cmd := exec.Command(cfPath, "tunnel", "route", "dns", tunnelName, hostname)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create DNS route: %w", err)
	}

	log.Printf("DNS route created: https://%s", hostname)
	return nil
}
