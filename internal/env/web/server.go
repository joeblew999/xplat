package web

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-via/via"
	"github.com/go-via/via-plugin-picocss/picocss"
	"github.com/joeblew999/xplat/internal/env"
)

// ServeSetupGUI starts the web server for environment setup
func ServeSetupGUI() error {
	serveSetupGUIWithOptions(false)
	return nil
}

// ServeSetupGUIMock starts the web server in mock mode (no real validation)
func ServeSetupGUIMock() error {
	serveSetupGUIWithOptions(true)
	return nil
}

// serveSetupGUIWithOptions is the internal implementation using Via
func serveSetupGUIWithOptions(mockMode bool) {
	// Ensure Caddy is running for HTTPS support
	if err := env.EnsureCaddyRunning(); err != nil {
		log.Printf("Warning: Failed to start Caddy (HTTPS will not be available): %v", err)
		log.Println("Continuing with HTTP-only mode...")
	}

	// Register Via GUI service with Caddy (event-based pattern)
	// Serve under /admin/* to avoid URL clashes with Hugo
	regResult, err := env.RegisterService(env.ServiceConfig{
		Name:          "via-gui",
		Port:          3000,
		PathPattern:   "/admin/*",    // Admin prefix to avoid Hugo clashes
		Priority:      10,            // Higher priority than Hugo
		HealthPath:    "/admin/",     // Health check endpoint
		AssetPatterns: []string{"/_*"}, // Via framework assets (/_plugins, /_datastar.js)
	})
	if err != nil {
		log.Printf("Warning: Failed to register Via GUI with Caddy: %v", err)
	}

	// Use test env file in mock mode
	if mockMode {
		env.SetEnvFileForTesting(env.GetTestEnvFile())
		defer env.ResetEnvFile()
	}

	log.Printf("\n")
	title := "Environment Setup GUI"
	if mockMode {
		title += " (Mock Mode)"
	}
	log.Println(title)
	if mockMode {
		log.Println("Mock validation enabled - no real API calls")
	}

	log.Println("Opening in browser...")
	// Use URLs from Caddy registration result
	if regResult != nil {
		log.Printf("\n  Local: %s", regResult.FullLocalURL)
		if regResult.FullLANURL != "" {
			log.Printf("\n  LAN:   %s", regResult.FullLANURL)
		}
	} else {
		// Fallback if registration failed
		log.Printf("\n  Local: %s", "https://localhost/admin/")
	}
	log.Printf("\n\n")
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\n\nShutting down gracefully...")
		log.Println("Unregistering Via GUI from Caddy...")
		if err := env.UnregisterService("via-gui"); err != nil {
			log.Printf("Warning: Failed to unregister Via GUI: %v\n", err)
		}
		log.Println("Stopping Caddy...")
		if err := env.StopCaddy(); err != nil {
			log.Printf("Warning: Failed to stop Caddy: %v\n", err)
		}
		log.Println("Stopping Hugo server...")
		env.StopHugoServer()
		log.Println("Goodbye!")
		os.Exit(0)
	}()

	v := via.New()
	v.Config(via.Options{
		DocumentTitle: "Environment Setup",
		Plugins:       []via.Plugin{picocss.Default},
		// DevMode enables the dataSPA Inspector debugging tool in the browser.
		// Set VIA_DEV_MODE=false to disable for production deployments.
		// Defaults to enabled when VIA_DEV_MODE is unset or any value other than "false".
		DevMode:       os.Getenv("VIA_DEV_MODE") != "false",
		LogLvl:        via.LogLevelWarn,  // Reduce noise from benign SSE race conditions
	})

	// Helper to load fresh config for each page request
	loadConfig := func() *env.EnvConfig {
		svc := env.NewService(mockMode)
		cfg, err := svc.GetCurrentConfig()
		if err != nil {
			log.Printf("Error loading config: %v", err)
			return &env.EnvConfig{}
		}
		return cfg
	}

	// Register routes - each loads fresh config
	v.Page("/", func(c *via.Context) {
		homePage(c, loadConfig(), mockMode)
	})

	// Cloudflare setup wizard - 5 steps
	v.Page("/cloudflare", func(c *via.Context) {
		cloudflarePage(c, loadConfig(), mockMode)
	})

	v.Page("/cloudflare/step1", func(c *via.Context) {
		cloudflareStep1Page(c, loadConfig(), mockMode)
	})

	v.Page("/cloudflare/step2", func(c *via.Context) {
		cloudflareStep2Page(c, loadConfig(), mockMode)
	})

	v.Page("/cloudflare/step3", func(c *via.Context) {
		cloudflareStep3Page(c, loadConfig(), mockMode)
	})

	v.Page("/cloudflare/step4", func(c *via.Context) {
		cloudflareStep4Page(c, loadConfig(), mockMode)
	})

	v.Page("/cloudflare/step5", func(c *via.Context) {
		cloudflareStep5Page(c, loadConfig(), mockMode)
	})

	v.Page("/claude", func(c *via.Context) {
		claudePage(c, loadConfig(), mockMode)
	})

	v.Page("/deploy", func(c *via.Context) {
		deployPage(c, loadConfig(), mockMode)
	})

	v.Start()
}
