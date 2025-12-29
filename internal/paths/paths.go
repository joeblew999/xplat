// Package paths provides wellknown xplat directory paths.
//
// xplat uses a two-tier directory system:
//
// 1. Global xplat home (~/.xplat/) - for cross-project binaries, config, cache
// 2. Project-local directories (.src/, .bin/, .data/) - for project-specific files
//
// Environment variables:
//   - XPLAT_HOME: Override global xplat home (default: ~/.xplat)
//   - PLAT_SRC: Project source directory (default: $PWD/.src)
//   - PLAT_BIN: Project binary directory (default: $PWD/.bin)
//   - PLAT_DATA: Project data directory (default: $PWD/.data)
//   - PLAT_DIST: Project dist directory (default: $PWD/.dist)
package paths

import (
	"os"
	"path/filepath"
)

// === Global xplat directories ===

// XplatHome returns the global xplat home directory.
// Uses XPLAT_HOME env var if set, otherwise ~/.xplat
func XplatHome() string {
	if h := os.Getenv("XPLAT_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".xplat"
	}
	return filepath.Join(home, ".xplat")
}

// XplatBin returns the global xplat binary directory.
// Binaries installed here are available across all projects.
// Returns ~/.xplat/bin (or $XPLAT_HOME/bin)
func XplatBin() string {
	return filepath.Join(XplatHome(), "bin")
}

// XplatCache returns the global xplat cache directory.
// Used for downloaded taskfiles, package caches, etc.
// Returns ~/.xplat/cache (or $XPLAT_HOME/cache)
func XplatCache() string {
	return filepath.Join(XplatHome(), "cache")
}

// XplatConfig returns the global xplat config directory.
// Used for user preferences, credentials, etc.
// Returns ~/.xplat/config (or $XPLAT_HOME/config)
func XplatConfig() string {
	return filepath.Join(XplatHome(), "config")
}

// === Project-local directories ===

// PlatSrc returns the project source directory for the given workdir.
// For cloned upstream source code.
func PlatSrc(workDir string) string {
	if v := os.Getenv("PLAT_SRC"); v != "" {
		return v
	}
	return filepath.Join(workDir, ".src")
}

// PlatBin returns the project binary directory for the given workdir.
// For built or downloaded binaries.
func PlatBin(workDir string) string {
	if v := os.Getenv("PLAT_BIN"); v != "" {
		return v
	}
	return filepath.Join(workDir, ".bin")
}

// PlatData returns the project data directory for the given workdir.
// For runtime data: databases, caches, logs.
func PlatData(workDir string) string {
	if v := os.Getenv("PLAT_DATA"); v != "" {
		return v
	}
	return filepath.Join(workDir, ".data")
}

// PlatDist returns the project dist directory for the given workdir.
// For release artifacts.
func PlatDist(workDir string) string {
	if v := os.Getenv("PLAT_DIST"); v != "" {
		return v
	}
	return filepath.Join(workDir, ".dist")
}

// === Environment setup ===

// SetPlatEnv sets the PLAT_* environment variables for a working directory.
// Call this before running tasks to inject the standard paths.
func SetPlatEnv(workDir string) {
	os.Setenv("PLAT_SRC", PlatSrc(workDir))
	os.Setenv("PLAT_BIN", PlatBin(workDir))
	os.Setenv("PLAT_DATA", PlatData(workDir))
	os.Setenv("PLAT_DIST", PlatDist(workDir))
}

// EnvSlice returns the PLAT_* environment variables as a slice for exec.Cmd.Env
func EnvSlice(workDir string) []string {
	return []string{
		"PLAT_SRC=" + PlatSrc(workDir),
		"PLAT_BIN=" + PlatBin(workDir),
		"PLAT_DATA=" + PlatData(workDir),
		"PLAT_DIST=" + PlatDist(workDir),
	}
}

// PathWithPlatBin returns a PATH string that includes PLAT_BIN and XPLAT_BIN.
// Use this when setting up PATH for subprocesses.
func PathWithPlatBin(workDir string) string {
	existingPath := os.Getenv("PATH")
	platBin := PlatBin(workDir)
	xplatBin := XplatBin()

	// Add both project-local and global xplat bins to PATH
	return platBin + ":" + xplatBin + ":" + existingPath
}

// FullEnv returns a complete environment slice including PLAT_* and updated PATH.
// This is the full environment needed for running tasks and services.
func FullEnv(workDir string) []string {
	env := os.Environ()

	// Add PLAT_* variables
	env = append(env, EnvSlice(workDir)...)

	// Update PATH to include PLAT_BIN and XPLAT_BIN
	newPath := PathWithPlatBin(workDir)
	for i, e := range env {
		if len(e) > 5 && e[:5] == "PATH=" {
			env[i] = "PATH=" + newPath
			return env
		}
	}
	// No PATH found, add it
	env = append(env, "PATH="+newPath)
	return env
}
