// Package config provides centralized configuration and paths for xplat.
//
// This package defines:
// - Default ports, paths, and behaviors used across xplat
// - Global xplat directories (~/.xplat/)
// - Project-local directories (.src/, .bin/, .data/, .dist/)
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
package config

import (
	"os"
	"path/filepath"
)

// === Default ports ===

const (
	// DefaultUIPort is the default port for web UIs (Task UI, Setup GUI, etc.).
	// Used by Via framework servers.
	DefaultUIPort = "3000"

	// DefaultUIPortInt is DefaultUIPort as an int for APIs that need it.
	DefaultUIPortInt = 3000

	// DefaultProcessComposePort is the default port for the process-compose API.
	DefaultProcessComposePort = 8080

	// DefaultWebhookPort is the default port for webhook servers.
	DefaultWebhookPort = "8080"

	// DefaultHugoPort is the default port for Hugo dev server.
	DefaultHugoPort = 1313

	// DefaultCaddyAdminPort is the default port for Caddy admin API.
	DefaultCaddyAdminPort = 2019
)

// === Default permissions ===

const (
	// DefaultDirPerms is the default permission mode for created directories.
	DefaultDirPerms = 0755

	// DefaultFilePerms is the default permission mode for created files.
	DefaultFilePerms = 0644
)

// === Default paths ===

const (
	// DefaultTaskfile is the default Taskfile path.
	DefaultTaskfile = "Taskfile.yml"

	// ProcessComposeGeneratedFile is the generated process-compose config file.
	// This is the primary output of `xplat manifest gen-process`.
	ProcessComposeGeneratedFile = "pc.generated.yaml"
)

// === Updater configuration ===

const (
	// XplatRepo is the GitHub repository for xplat releases.
	XplatRepo = "joeblew999/xplat"

	// XplatReleasesAPI is the GitHub API endpoint for the latest xplat release.
	XplatReleasesAPI = "https://api.github.com/repos/" + XplatRepo + "/releases/latest"

	// XplatChecksumFile is the name of the checksum file in releases.
	XplatChecksumFile = "checksums.txt"

	// XplatTagPrefix is the prefix for xplat release tags (e.g., "xplat-v0.3.0").
	XplatTagPrefix = "xplat-"
)

// ProcessComposeSearchOrder returns the order to search for process-compose config files.
// Generated files are prioritized over manual files, and short names over long names.
func ProcessComposeSearchOrder() []string {
	return []string{
		"pc.generated.yaml",
		"pc.yaml",
		"pc.yml",
		"process-compose.generated.yaml",
		"process-compose.yaml",
		"process-compose.yml",
	}
}

// === Default behaviors ===

const (
	// DefaultOpenBrowser controls whether to open browser on UI start.
	DefaultOpenBrowser = true
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

// XplatProjects returns the path to the local project registry.
// This file tracks all registered xplat projects on this machine.
// Returns ~/.xplat/projects.yaml (or $XPLAT_HOME/projects.yaml)
func XplatProjects() string {
	return filepath.Join(XplatHome(), "projects.yaml")
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

// PathWithPlatBin returns a PATH string that includes PLAT_BIN.
// Use this when setting up PATH for subprocesses.
// Uses the platform-appropriate path separator (: on Unix, ; on Windows).
//
// Note: We only prepend PLAT_BIN (project-local), NOT XPLAT_BIN.
// The user's xplat installation location (from their PATH) should be respected.
// This prevents version conflicts when multiple xplat versions exist.
func PathWithPlatBin(workDir string) string {
	existingPath := os.Getenv("PATH")
	platBin := PlatBin(workDir)

	// Only add project-local bin to PATH
	// Use filepath.ListSeparator for platform compatibility
	sep := string(filepath.ListSeparator)
	return platBin + sep + existingPath
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
