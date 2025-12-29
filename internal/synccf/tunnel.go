package synccf

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
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

// CheckCloudflared verifies cloudflared is installed
func CheckCloudflared() error {
	cmd := exec.Command("cloudflared", "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cloudflared not found: %w (install from https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation)", err)
	}
	log.Printf("sync-cf: found cloudflared %s", strings.TrimSpace(string(output)))
	return nil
}

// InstallCloudflared attempts to install cloudflared
func InstallCloudflared() error {
	switch {
	case fileExists("/opt/homebrew/bin/brew") || fileExists("/usr/local/bin/brew"):
		cmd := exec.Command("brew", "install", "cloudflared")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("automatic installation not supported for this OS, please install manually from https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
