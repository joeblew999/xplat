package osutil

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/bmatcuk/doublestar/v4"
)

// Glob expands a glob pattern and returns matching file paths.
// Supports doublestar (**) patterns for recursive matching.
// On Windows, matching is case-insensitive.
func Glob(pattern string) ([]string, error) {
	var opts []doublestar.GlobOption
	if runtime.GOOS == "windows" {
		opts = append(opts, doublestar.WithNoFollow())
	}

	if filepath.IsAbs(pattern) {
		return doublestar.FilepathGlob(pattern, opts...)
	}

	return doublestar.Glob(os.DirFS("."), pattern, opts...)
}

// GlobIn expands a glob pattern relative to a base directory.
func GlobIn(baseDir, pattern string) ([]string, error) {
	var opts []doublestar.GlobOption
	if runtime.GOOS == "windows" {
		opts = append(opts, doublestar.WithNoFollow())
	}

	matches, err := doublestar.Glob(os.DirFS(baseDir), pattern, opts...)
	if err != nil {
		return nil, err
	}

	// Convert to absolute paths
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = filepath.Join(baseDir, m)
	}
	return result, nil
}
