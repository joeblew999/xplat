package osutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Env gets an environment variable value. If the variable is not set,
// returns the default value. If no default is provided and the variable
// is not set, returns an empty string.
func Env(key string, defaultValue ...string) string {
	value, exists := os.LookupEnv(key)
	if !exists && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// EnvExists returns true if the environment variable is set.
func EnvExists(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// Which finds the path to an executable in PATH.
// Returns empty string if not found.
func Which(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// ExeExt returns the executable extension for the current platform.
// Returns ".exe" on Windows, empty string otherwise.
func ExeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// HomeDir returns the user's home directory.
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ExpandHome expands ~ and $HOME in a path to the actual home directory.
func ExpandHome(path string) string {
	home := HomeDir()
	if home == "" {
		return path
	}
	path = strings.ReplaceAll(path, "~", home)
	path = strings.ReplaceAll(path, "$HOME", home)
	path = strings.ReplaceAll(path, "{{.HOME}}", home)
	return path
}

// PathJoin joins path elements using the correct separator for the current OS.
func PathJoin(elem ...string) string {
	return filepath.Join(elem...)
}

// ToSlash converts path separators to forward slashes.
// Useful for URLs and cross-platform path strings.
func ToSlash(path string) string {
	return filepath.ToSlash(path)
}

// FromSlash converts forward slashes to the native path separator.
func FromSlash(path string) string {
	return filepath.FromSlash(path)
}
