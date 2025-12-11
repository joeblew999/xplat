package env

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	caddyPort          = "443"
	caddyCertDir       = ".caddy/certs"
	caddyfilePath      = ".caddy/Caddyfile"
	serviceRegistryPath = ".caddy/services.json"
	caddyAdminAPIURL   = "http://localhost:2019" // Caddy admin API endpoint

	// Timeout constants
	caddyStartupTimeout      = 2 * time.Second         // Time to wait for Caddy admin API to become available
	caddyHealthCheckTimeout  = 500 * time.Millisecond  // Timeout for health check connections
	caddyShutdownGracePeriod = 500 * time.Millisecond  // Time to wait after sending stop request
	caddyAdminAPITimeout     = 2 * time.Second         // Timeout for admin API HTTP requests

	// File permission constants
	defaultDirPerms  = 0755 // Directory permissions for .caddy directories
	defaultFilePerms = 0644 // File permissions for config files and certificates
)

// ServiceConfig represents a service to be reverse-proxied by Caddy
type ServiceConfig struct {
	Name          string   `json:"name"`           // Unique service identifier (e.g., "via-gui", "hugo")
	Port          int      `json:"port"`           // Local port service listens on
	PathPattern   string   `json:"path_pattern"`   // URL path pattern (e.g., "/admin/*" or "" for root)
	Priority      int      `json:"priority"`       // Routing priority (higher = earlier in config, default: 0)
	HealthPath    string   `json:"health_path"`    // Optional health check path (e.g., "/", "/admin/")
	AssetPatterns []string `json:"asset_patterns"` // Optional static asset path patterns (e.g., ["*.js", "*.css", "*.png"])
}

// ServiceRegistrationResult contains information returned to services after registration
// This allows services to configure themselves based on their HTTPS URLs and routing
type ServiceRegistrationResult struct {
	LocalBaseURL string // Local HTTPS base URL (e.g., "https://localhost/admin")
	LANBaseURL   string // LAN HTTPS base URL (e.g., "https://192.168.1.49/admin")
	BasePath     string // The base path for this service (e.g., "/admin" or "/")
	FullLocalURL string // Full local URL with health path (e.g., "https://localhost/admin/")
	FullLANURL   string // Full LAN URL with health path (e.g., "https://192.168.1.49/admin/")
}

// serviceRegistry manages the list of registered services
type serviceRegistry struct {
	Services []ServiceConfig `json:"services"`
}

// registryManager provides thread-safe access to the service registry
type registryManager struct {
	mu sync.Mutex
}

// Global registry manager instance
var globalRegistryManager = &registryManager{}

// Global Caddy process management
var (
	caddyProcessCmd *exec.Cmd  // Track the running Caddy process
	caddyProcessMux sync.Mutex // Protect access to process handle
)

// load reads the service registry from disk (private, not locked)
func (rm *registryManager) load() (*serviceRegistry, error) {
	// If file doesn't exist, return empty registry
	if !fileExists(serviceRegistryPath) {
		return &serviceRegistry{Services: []ServiceConfig{}}, nil
	}

	data, err := os.ReadFile(serviceRegistryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service registry: %w", err)
	}

	var registry serviceRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse service registry: %w", err)
	}

	return &registry, nil
}

// save writes the service registry to disk (private, not locked)
func (rm *registryManager) save(registry *serviceRegistry) error {
	// Ensure .caddy directory exists
	caddyDir := filepath.Dir(serviceRegistryPath)
	if err := os.MkdirAll(caddyDir, defaultDirPerms); err != nil {
		return fmt.Errorf("failed to create .caddy directory: %w", err)
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service registry: %w", err)
	}

	if err := os.WriteFile(serviceRegistryPath, data, defaultFilePerms); err != nil {
		return fmt.Errorf("failed to write service registry: %w", err)
	}

	return nil
}

// EnsureCaddyRunning ensures Caddy is running with proper HTTPS configuration.
// This function is idempotent - safe to call multiple times.
// Starts Caddy with the current service registry (may be empty).
func EnsureCaddyRunning() error {
	// Check if Caddy is already running
	if IsCaddyRunning() {
		fmt.Println("✓ Caddy is already running on port 443")
		return nil
	}

	// Ensure certificates are ready
	if err := ensureCerts(); err != nil {
		return fmt.Errorf("failed to ensure certificates: %w", err)
	}

	// Start Caddy with current service registry
	if err := StartCaddy(); err != nil {
		return fmt.Errorf("failed to start Caddy: %w", err)
	}

	fmt.Println("✓ Caddy started successfully on port 443")
	return nil
}

// IsCaddyRunning checks if Caddy process is running by checking admin API availability
func IsCaddyRunning() bool {
	return IsAdminAPIAvailable()
}

// getUserHomeDir returns the user's home directory in a cross-platform way
// Checks USERPROFILE on Windows, HOME on Unix, with os.UserHomeDir as fallback
func getUserHomeDir() string {
	// Try platform-specific environment variables first
	if runtime.GOOS == "windows" {
		if home := os.Getenv("USERPROFILE"); home != "" {
			return home
		}
	} else {
		if home := os.Getenv("HOME"); home != "" {
			return home
		}
	}

	// Fallback to os.UserHomeDir (Go 1.12+)
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	// Last resort fallback
	return "."
}

// getBinaryName returns the binary name with platform-specific extension
// Appends .exe on Windows, returns base name on other platforms
func getBinaryName(baseName string) string {
	if runtime.GOOS == "windows" {
		return baseName + ".exe"
	}
	return baseName
}

// getCaddyBinaryPath returns the absolute path to the Caddy binary
// Checks in order: CADDY_BIN env var, system PATH, cross-platform fallback
func getCaddyBinaryPath() string {
	// 1. Check environment variable (allows user override)
	if bin := os.Getenv("CADDY_BIN"); bin != "" {
		return bin
	}

	// 2. Try to find in system PATH
	binaryName := getBinaryName("caddy")
	if bin, err := exec.LookPath(binaryName); err == nil {
		return bin
	}

	// 3. Fall back to cross-platform home directory path
	homeDir := getUserHomeDir()
	return filepath.Join(homeDir, "workspace", "go", "bin", binaryName)
}

// getMkcertBinaryPath returns the absolute path to the mkcert binary
// Checks in order: MKCERT_BIN env var, system PATH, cross-platform home directory
func getMkcertBinaryPath() string {
	// 1. Check environment variable (allows user override)
	if bin := os.Getenv("MKCERT_BIN"); bin != "" {
		return bin
	}

	// 2. Try to find in system PATH
	binaryName := getBinaryName("mkcert")
	if bin, err := exec.LookPath(binaryName); err == nil {
		return bin
	}

	// 3. Fall back to cross-platform home directory path
	homeDir := getUserHomeDir()
	return filepath.Join(homeDir, "workspace", "go", "bin", binaryName)
}


// StartCaddy starts the Caddy server with current service registry configuration
func StartCaddy() error {
	// Load current service registry (read-only, no lock needed for startup)
	registry, err := globalRegistryManager.load()
	if err != nil {
		return fmt.Errorf("failed to load service registry: %w", err)
	}

	// Get LAN IP
	lanIP, err := GetLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}

	// Generate Caddyfile from services
	caddyfileContent := generateCaddyfileFromServices(lanIP, registry.Services)

	// Ensure .caddy directory exists
	caddyDir := filepath.Dir(caddyfilePath)
	if err := os.MkdirAll(caddyDir, defaultDirPerms); err != nil {
		return fmt.Errorf("failed to create .caddy directory: %w", err)
	}

	// Write Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte(caddyfileContent), defaultFilePerms); err != nil {
		return fmt.Errorf("failed to write Caddyfile: %w", err)
	}

	// Find caddy binary
	caddyBin := getCaddyBinaryPath()
	if _, err := os.Stat(caddyBin); os.IsNotExist(err) {
		return fmt.Errorf("caddy binary not found at %s\n"+
			"Install: https://caddyserver.com/docs/install\n"+
			"Or set CADDY_BIN environment variable to custom path", caddyBin)
	}

	// Start Caddy in background
	cmd := exec.Command(caddyBin, "run", "--config", caddyfilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start caddy: %w", err)
	}

	// Store process handle for graceful shutdown
	caddyProcessMux.Lock()
	caddyProcessCmd = cmd
	caddyProcessMux.Unlock()

	// Give Caddy a moment to start (using correct timeout constant)
	time.Sleep(caddyStartupTimeout)

	// Verify it started
	if !IsCaddyRunning() {
		return fmt.Errorf("caddy failed to start")
	}

	fmt.Printf("✓ Caddy started with HTTPS for localhost and %s\n", lanIP)
	return nil
}

// StopCaddy stops the Caddy server gracefully using admin API
func StopCaddy() error {
	if !IsCaddyRunning() {
		fmt.Println("Caddy is not running")
		// Clear process handle if any
		caddyProcessMux.Lock()
		caddyProcessCmd = nil
		caddyProcessMux.Unlock()
		return nil
	}

	// Use admin API to stop Caddy gracefully
	client := &http.Client{
		Timeout: caddyAdminAPITimeout,
	}

	req, err := http.NewRequest("POST", caddyAdminAPIURL+"/stop", nil)
	if err != nil {
		return fmt.Errorf("failed to create stop request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send stop request to Caddy admin API: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("admin API returned error status: %d", resp.StatusCode)
	}

	// Clear process handle
	caddyProcessMux.Lock()
	caddyProcessCmd = nil
	caddyProcessMux.Unlock()

	// Give Caddy a moment to shut down
	time.Sleep(caddyShutdownGracePeriod)

	// Verify Caddy stopped
	if IsCaddyRunning() {
		return fmt.Errorf("Caddy is still running after stop request")
	}

	fmt.Println("✓ Caddy stopped")
	return nil
}

// IsAdminAPIAvailable checks if Caddy admin API is available
func IsAdminAPIAvailable() bool {
	// Try to connect to admin API (strip http:// prefix for TCP dial)
	adminAddr := strings.TrimPrefix(caddyAdminAPIURL, "http://")
	conn, err := net.DialTimeout("tcp", adminAddr, caddyHealthCheckTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// CheckServiceHealth checks if a service is responding on its configured port
// Returns true if service responds with HTTP 200, false otherwise
func CheckServiceHealth(service ServiceConfig) bool {
	// Check if port is listening
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", service.Port), caddyHealthCheckTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// CheckHTTPSHealth checks if a service is accessible via HTTPS through Caddy
// Returns HTTP status code and error (0 if connection failed)
func CheckHTTPSHealth(httpsURL string) (int, error) {
	// Create HTTP client that skips TLS verification (self-signed certs)
	client := &http.Client{
		Timeout: caddyAdminAPITimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(httpsURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// ReloadCaddy reloads Caddy configuration without restarting using admin API
func ReloadCaddy() error {
	if !IsAdminAPIAvailable() {
		return fmt.Errorf("Caddy admin API not available at %s", caddyAdminAPIURL)
	}

	caddyBin := getCaddyBinaryPath()
	adminAddr := strings.TrimPrefix(caddyAdminAPIURL, "http://")
	cmd := exec.Command(caddyBin, "reload", "--config", caddyfilePath, "--adapter", "caddyfile", "--address", adminAddr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload Caddy configuration: %w", err)
	}

	fmt.Printf("✓ Caddy configuration reloaded via admin API\n")
	return nil
}

// generateCaddyfileFromServices creates a Caddyfile from registered services
// Services are sorted by priority (higher first), then by whether they have a path pattern
func generateCaddyfileFromServices(lanIP string, services []ServiceConfig) string {
	// Get absolute path to certs
	certPath := filepath.Join(caddyCertDir, "cert.pem")
	keyPath := filepath.Join(caddyCertDir, "key.pem")

	// Start with global config
	var sb strings.Builder
	sb.WriteString(`{
	auto_https off
	admin ` + strings.TrimPrefix(caddyAdminAPIURL, "http://") + `
}

https://localhost:443, https://`)
	sb.WriteString(lanIP)
	sb.WriteString(`:443, https://*.local:443 {
	tls `)
	sb.WriteString(certPath)
	sb.WriteString(` `)
	sb.WriteString(keyPath)
	sb.WriteString("\n\n")

	// Sort services: higher priority first, then path-based before root
	sortedServices := make([]ServiceConfig, len(services))
	copy(sortedServices, services)

	// Sort services by priority (higher first) and path pattern (paths before root)
	sort.Slice(sortedServices, func(i, j int) bool {
		// Higher priority comes first
		if sortedServices[i].Priority != sortedServices[j].Priority {
			return sortedServices[i].Priority > sortedServices[j].Priority
		}
		// Same priority: path-based routes before root routes
		iPat := sortedServices[i].PathPattern
		jPat := sortedServices[j].PathPattern
		if iPat != "" && jPat == "" {
			return true
		}
		if iPat == "" && jPat != "" {
			return false
		}
		return false // Both have paths or both are root - maintain stable order
	})

	// Add service routes
	if len(sortedServices) == 0 {
		// No services - just a placeholder comment
		sb.WriteString("\t# No services registered\n")
	} else {
		for _, svc := range sortedServices {
			if svc.PathPattern != "" {
				// Path-based routing with URI prefix stripping
				// Generate comment with service details
				sb.WriteString("\t# ")
				sb.WriteString(svc.Name)
				sb.WriteString(" (priority: ")
				sb.WriteString(fmt.Sprintf("%d", svc.Priority))
				sb.WriteString(")\n")

				// Add URL mapping
				sb.WriteString("\t# HTTPS: https://localhost")
				// Strip trailing /* for display
				displayPath := strings.TrimSuffix(svc.PathPattern, "/*")
				if displayPath == "" {
					displayPath = "/"
				}
				sb.WriteString(displayPath)
				sb.WriteString("/ → http://localhost:")
				sb.WriteString(fmt.Sprintf("%d", svc.Port))
				sb.WriteString("/\n")

				// Add note about URI prefix stripping
				stripPrefix := strings.TrimSuffix(displayPath, "/")
				sb.WriteString("\t# Note: URI prefix stripped (")
				sb.WriteString(stripPrefix)
				sb.WriteString(") before proxying\n")

				sb.WriteString("\thandle ")
				sb.WriteString(svc.PathPattern)
				sb.WriteString(" {\n\t\turi strip_prefix ")
				sb.WriteString(stripPrefix)
				sb.WriteString("\n\t\treverse_proxy localhost:")
				sb.WriteString(fmt.Sprintf("%d", svc.Port))
				sb.WriteString("\n\t}\n\n")

				// Add routes for any declared asset patterns (e.g., framework assets)
				// Services can declare patterns like ["/_*"] for root-relative assets
				if len(svc.AssetPatterns) > 0 {
					sb.WriteString("\t# ")
					sb.WriteString(svc.Name)
					sb.WriteString(" static assets\n")
					for _, pattern := range svc.AssetPatterns {
						sb.WriteString("\thandle ")
						sb.WriteString(pattern)
						sb.WriteString(" {\n")
						sb.WriteString("\t\treverse_proxy localhost:")
						sb.WriteString(fmt.Sprintf("%d", svc.Port))
						sb.WriteString("\n\t}\n")
					}
					sb.WriteString("\n")
				}

			} else {
				// Root routing (catch-all)
				sb.WriteString("\t# ")
				sb.WriteString(svc.Name)
				sb.WriteString(" (priority: ")
				sb.WriteString(fmt.Sprintf("%d", svc.Priority))
				sb.WriteString(", root catch-all)\n")

				// Add URL mapping
				sb.WriteString("\t# HTTPS: https://localhost/ → http://localhost:")
				sb.WriteString(fmt.Sprintf("%d", svc.Port))
				sb.WriteString("\n")

				sb.WriteString("\thandle {\n\t\treverse_proxy localhost:")
				sb.WriteString(fmt.Sprintf("%d", svc.Port))
				sb.WriteString("\n\t}\n\n")
			}
		}
	}

	// Add logging
	sb.WriteString(`	# Enable logging for debugging
	log {
		output file .caddy/access.log
		format console
	}
}
`)

	return sb.String()
}

// ensureCerts ensures mkcert certificates exist and are valid for current network
func ensureCerts() error {
	certPath := filepath.Join(caddyCertDir, "cert.pem")
	keyPath := filepath.Join(caddyCertDir, "key.pem")

	// Get current LAN IP
	lanIP, err := GetLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}

	// Check if certificates exist
	certExists := fileExists(certPath) && fileExists(keyPath)

	needsRegeneration := false

	if certExists {
		// Parse existing certificate and check if LAN IP is in SAN
		hasIP, err := certContainsIP(certPath, lanIP)
		if err != nil {
			fmt.Printf("Warning: failed to parse existing certificate: %v\n", err)
			needsRegeneration = true
		} else if !hasIP {
			fmt.Printf("Current LAN IP %s not in certificate, regenerating...\n", lanIP)
			needsRegeneration = true
		} else {
			fmt.Printf("✓ Certificate already valid for %s\n", lanIP)
			return nil
		}
	} else {
		fmt.Println("No certificates found, generating new ones...")
		needsRegeneration = true
	}

	if needsRegeneration {
		return generateCerts(lanIP, certPath, keyPath)
	}

	return nil
}

// certContainsIP checks if the certificate contains the given IP in its SAN
func certContainsIP(certPath string, ip string) (bool, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return false, err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, err
	}

	// Check IP addresses in SAN
	targetIP := net.ParseIP(ip)
	for _, certIP := range cert.IPAddresses {
		if certIP.Equal(targetIP) {
			return true, nil
		}
	}

	return false, nil
}

// generateCerts generates new mkcert certificates
func generateCerts(lanIP string, certPath string, keyPath string) error {
	// Ensure cert directory exists
	if err := os.MkdirAll(caddyCertDir, 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Find mkcert binary
	mkcertBin := getMkcertBinaryPath()
	if _, err := os.Stat(mkcertBin); os.IsNotExist(err) {
		return fmt.Errorf("mkcert binary not found at %s\n"+
			"Install: https://github.com/FiloSottile/mkcert#installation\n"+
			"Or set MKCERT_BIN environment variable to custom path", mkcertBin)
	}

	// Install mkcert CA if not already installed
	installCmd := exec.Command(mkcertBin, "-install")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		fmt.Printf("Warning: mkcert -install failed: %v\n", err)
	}

	// Generate certificate
	fmt.Printf("Generating certificates for localhost, *.local, and %s...\n", lanIP)

	cmd := exec.Command(
		mkcertBin,
		"-cert-file", certPath,
		"-key-file", keyPath,
		"localhost",
		"*.local",
		lanIP,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate certificates: %w", err)
	}

	fmt.Printf("✓ Certificates generated successfully\n")
	fmt.Printf("  - Certificate: %s\n", certPath)
	fmt.Printf("  - Key: %s\n", keyPath)

	return nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetCaddyVersion returns the Caddy version and binary location
func GetCaddyVersion() (version string, binaryPath string, err error) {
	binaryPath = getCaddyBinaryPath()

	// Check if binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "", binaryPath, fmt.Errorf("caddy binary not found at %s\n"+
			"Install: https://caddyserver.com/docs/install\n"+
			"Or set CADDY_BIN environment variable to custom path", binaryPath)
	}

	// Get version
	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", binaryPath, fmt.Errorf("failed to get caddy version: %w", err)
	}

	version = strings.TrimSpace(string(output))
	return version, binaryPath, nil
}

// PrintCaddyVersion prints Caddy version and binary location
func PrintCaddyVersion() {
	version, binaryPath, err := GetCaddyVersion()
	if err != nil {
		fmt.Printf("Caddy: %v\n", err)
		return
	}
	fmt.Printf("Caddy:\n")
	fmt.Printf("  Binary: %s\n", binaryPath)
	fmt.Printf("  Version: %s\n", version)
}

// PrintCaddyStatus prints comprehensive Caddy status information
func PrintCaddyStatus() error {
	// Check if running
	if !IsCaddyRunning() {
		fmt.Println("✗ Caddy is not running")
		return nil
	}

	fmt.Println("✓ Caddy is running on port 443")
	fmt.Println()

	// Get version
	version, binaryPath, err := GetCaddyVersion()
	if err == nil {
		fmt.Printf("Binary: %s\n", binaryPath)
		fmt.Printf("Version: %s\n", version)
	}

	// Get LAN IP
	lanIP, err := GetLocalIP()
	if err == nil {
		fmt.Printf("LAN IP: %s\n", lanIP)
	}

	// Show admin endpoint status
	if IsAdminAPIAvailable() {
		fmt.Printf("Admin API: %s (✓ available)\n", caddyAdminAPIURL)
	} else {
		fmt.Printf("Admin API: not available\n")
	}

	fmt.Println()

	// Get registered services
	services, err := GetRegisteredServices()
	if err != nil {
		return fmt.Errorf("failed to get registered services: %w", err)
	}

	if len(services) == 0 {
		fmt.Println("Registered Services: none")
	} else {
		fmt.Printf("Registered Services: %d\n", len(services))

		// Get LAN IP for displaying LAN URLs (empty string if not available)
		lanIP := GetLocalIPOrFallback()

		for _, svc := range services {
			// Determine base path
			basePath := svc.PathPattern
			if basePath == "" {
				basePath = "/"
			} else {
				// Strip trailing /* from pattern for display
				basePath = strings.TrimSuffix(basePath, "/*")
				if basePath == "" {
					basePath = "/"
				}
			}

			// Build service display line with Local and LAN URLs
			var serviceLine string
			localURL := "https://localhost" + basePath
			if svc.PathPattern != "" {
				serviceLine = fmt.Sprintf("  • %s: %s → localhost:%d (priority: %d)",
					svc.Name, localURL, svc.Port, svc.Priority)
			} else {
				serviceLine = fmt.Sprintf("  • %s: %s → localhost:%d (priority: %d, root)",
					svc.Name, localURL, svc.Port, svc.Priority)
			}

			// Check health and add status indicator
			portHealth := CheckServiceHealth(svc)
			if portHealth {
				serviceLine += " ✓"
			} else {
				serviceLine += " ✗ (port not responding)"
			}

			// If health path is configured, also check HTTPS endpoint
			if svc.HealthPath != "" && portHealth {
				httpsURL := "https://localhost" + svc.HealthPath
				statusCode, err := CheckHTTPSHealth(httpsURL)
				if err == nil && statusCode >= 200 && statusCode < 400 {
					serviceLine += fmt.Sprintf(" [HTTPS: %d]", statusCode)
				} else if err != nil {
					serviceLine += " [HTTPS: error]"
				} else {
					serviceLine += fmt.Sprintf(" [HTTPS: %d]", statusCode)
				}
			}

			fmt.Println(serviceLine)

			// Print LAN URL if available
			if lanIP != "" {
				lanURL := fmt.Sprintf("https://%s%s", lanIP, basePath)
				fmt.Printf("    LAN: %s\n", lanURL)
			}
		}
	}

	return nil
}

// validateServiceConfig validates service configuration parameters
func validateServiceConfig(service ServiceConfig) error {
	// Validate service name (alphanumeric, hyphens, underscores only)
	if service.Name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	for _, char := range service.Name {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
		     (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return fmt.Errorf("service name '%s' contains invalid characters (use alphanumeric, hyphens, underscores only)", service.Name)
		}
	}

	// Validate port range
	if service.Port < 1 || service.Port > 65535 {
		return fmt.Errorf("port %d out of valid range (1-65535)", service.Port)
	}

	// Validate path pattern (must start with / or be empty)
	if service.PathPattern != "" && !strings.HasPrefix(service.PathPattern, "/") {
		return fmt.Errorf("path pattern '%s' must start with '/' or be empty", service.PathPattern)
	}

	// Validate health path (must start with / or be empty)
	if service.HealthPath != "" && !strings.HasPrefix(service.HealthPath, "/") {
		return fmt.Errorf("health path '%s' must start with '/' or be empty", service.HealthPath)
	}

	// Validate AssetPatterns (must be safe and properly formatted)
	for _, pattern := range service.AssetPatterns {
		if pattern == "" {
			return fmt.Errorf("asset pattern cannot be empty")
		}
		if !strings.HasPrefix(pattern, "/") {
			return fmt.Errorf("asset pattern '%s' must start with '/'", pattern)
		}
		if strings.Contains(pattern, "..") {
			return fmt.Errorf("asset pattern '%s' contains path traversal (..) which is not allowed", pattern)
		}
		// Warn about overly broad patterns (informational only, not an error)
		if pattern == "/*" {
			fmt.Printf("Warning: asset pattern '/*' is very broad and may conflict with other routes\n")
		}
	}

	return nil
}

// RegisterService registers a service with Caddy and reloads the configuration.
// This function is idempotent - re-registering the same service will not cause a reload.
// Returns ServiceRegistrationResult with the service's base URLs and configuration info.
func RegisterService(service ServiceConfig) (*ServiceRegistrationResult, error) {
	// Validate service configuration
	if err := validateServiceConfig(service); err != nil {
		return nil, fmt.Errorf("invalid service config: %w", err)
	}

	// Acquire lock for thread-safe registry access
	globalRegistryManager.mu.Lock()
	defer globalRegistryManager.mu.Unlock()

	// Load current registry
	registry, err := globalRegistryManager.load()
	if err != nil {
		return nil, fmt.Errorf("failed to load service registry: %w", err)
	}

	// Check if service already registered with same config
	for i, existing := range registry.Services {
		if existing.Name == service.Name {
			// Service exists - check if config changed
			if existing.Port == service.Port &&
			   existing.PathPattern == service.PathPattern &&
			   existing.Priority == service.Priority &&
			   existing.HealthPath == service.HealthPath {
				// Same config, no-op (idempotent)
				fmt.Printf("✓ Service '%s' already registered\n", service.Name)
				return buildRegistrationResult(service), nil
			}
			// Config changed - update it
			registry.Services[i] = service
			fmt.Printf("✓ Service '%s' config updated\n", service.Name)
			err := reloadCaddyWithServices(registry)
			if err != nil {
				return nil, err
			}
			return buildRegistrationResult(service), nil
		}
	}

	// New service - add it
	registry.Services = append(registry.Services, service)
	fmt.Printf("✓ Service '%s' registered on port %d\n", service.Name, service.Port)
	err = reloadCaddyWithServices(registry)
	if err != nil {
		return nil, err
	}
	return buildRegistrationResult(service), nil
}

// buildRegistrationResult creates a ServiceRegistrationResult from a ServiceConfig
func buildRegistrationResult(service ServiceConfig) *ServiceRegistrationResult {
	// Determine base path
	basePath := service.PathPattern
	if basePath == "" {
		basePath = "/"
	} else {
		// Strip trailing /* from pattern for clean base path
		basePath = strings.TrimSuffix(basePath, "/*")
		if basePath == "" {
			basePath = "/"
		}
	}

	// Get LAN IP (empty string if not available)
	lanIP := GetLocalIPOrFallback()

	// Build URLs
	result := &ServiceRegistrationResult{
		LocalBaseURL: "https://localhost" + basePath,
		BasePath:     basePath,
	}

	if lanIP != "" {
		result.LANBaseURL = fmt.Sprintf("https://%s%s", lanIP, basePath)
	}

	// Add full URLs with health path if available
	if service.HealthPath != "" {
		result.FullLocalURL = "https://localhost" + service.HealthPath
		if lanIP != "" {
			result.FullLANURL = fmt.Sprintf("https://%s%s", lanIP, service.HealthPath)
		}
	} else {
		result.FullLocalURL = result.LocalBaseURL
		result.FullLANURL = result.LANBaseURL
	}

	return result
}

// UnregisterService removes a service from Caddy and reloads the configuration.
// This function is idempotent - unregistering a non-existent service is a no-op.
func UnregisterService(serviceName string) error {
	// Acquire lock for thread-safe registry access
	globalRegistryManager.mu.Lock()
	defer globalRegistryManager.mu.Unlock()

	// Load current registry
	registry, err := globalRegistryManager.load()
	if err != nil {
		return fmt.Errorf("failed to load service registry: %w", err)
	}

	// Find and remove service
	found := false
	newServices := make([]ServiceConfig, 0, len(registry.Services))
	for _, existing := range registry.Services {
		if existing.Name == serviceName {
			found = true
			fmt.Printf("✓ Service '%s' unregistered\n", serviceName)
			continue // Skip this service
		}
		newServices = append(newServices, existing)
	}

	if !found {
		// Service not found, no-op (idempotent)
		fmt.Printf("✓ Service '%s' not registered (already removed)\n", serviceName)
		return nil
	}

	registry.Services = newServices
	return reloadCaddyWithServices(registry)
}

// GetRegisteredServices returns the list of currently registered services
func GetRegisteredServices() ([]ServiceConfig, error) {
	// Acquire lock for thread-safe registry access
	globalRegistryManager.mu.Lock()
	defer globalRegistryManager.mu.Unlock()

	registry, err := globalRegistryManager.load()
	if err != nil {
		return nil, err
	}
	return registry.Services, nil
}

// reloadCaddyWithServices regenerates Caddyfile from services and reloads Caddy
func reloadCaddyWithServices(registry *serviceRegistry) error {
	// Save registry to disk (Note: mutex should be held by caller)
	if err := globalRegistryManager.save(registry); err != nil {
		return err
	}

	// Get LAN IP for Caddyfile
	lanIP, err := GetLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}

	// Generate Caddyfile from services
	caddyfileContent := generateCaddyfileFromServices(lanIP, registry.Services)

	// Write Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte(caddyfileContent), 0644); err != nil {
		return fmt.Errorf("failed to write Caddyfile: %w", err)
	}

	// Reload Caddy if running
	if IsCaddyRunning() {
		return ReloadCaddy()
	}

	// Caddy not running - no reload needed
	return nil
}
