// Package bootstrap initializes logging configuration before other packages.
//
// This package MUST be imported first (using a blank import) in main.go to ensure
// its init() runs before other packages that use zerolog, particularly process-compose.
//
// Go's initialization order:
//  1. Imported packages initialize in dependency order (depth-first)
//  2. Within a package, files are sorted by name, init() runs in order
//  3. The main package initializes last
//
// By importing this package before the cmd package (which imports process-compose),
// we can set zerolog's global level to info before process-compose's init() runs,
// suppressing its debug-level "could not locate process-compose config" messages.
package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/rs/zerolog"
)

func init() {
	// Check if user has explicitly set log level
	level := os.Getenv("PC_LOG_LEVEL")
	if level == "" {
		level = "info"
		_ = os.Setenv("PC_LOG_LEVEL", level)
	}

	// Set zerolog's global level directly to suppress debug logs during init
	// Parse the level to respect user's setting (e.g., PC_LOG_LEVEL=debug)
	logLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	// Check for stale xplat binaries in non-canonical locations
	checkStaleBinaries()
}

// checkStaleBinaries warns if xplat exists in locations other than ~/.local/bin
func checkStaleBinaries() {
	canonical := config.XplatCanonicalBin()

	// Get current executable path
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)

	for _, loc := range config.XplatStaleLocations() {
		if _, err := os.Stat(loc); err == nil {
			// Stale binary exists
			resolved, _ := filepath.EvalSymlinks(loc)
			if resolved != exe && loc != canonical {
				fmt.Fprintf(os.Stderr, "⚠️  Stale xplat at %s (run: rm %s)\n", loc, loc)
			}
		}
	}
}
