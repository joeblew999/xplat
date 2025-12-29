// Package osutil provides cross-platform file system utilities for CLI commands.
//
// IMPORTANT: This package is designed ONLY for use by CLI commands in cmd/xplat/cmd/.
// These commands are called from Taskfiles to provide Unix-like utilities on Windows
// where commands like "mkdir -p", "rm -rf", "touch", etc. don't exist natively.
//
// DO NOT use this package in other internal packages! Go's standard library (os.Stat,
// os.MkdirAll, os.Remove, etc.) already works cross-platform. This package exists
// solely to provide shell-command-like behavior for Taskfile compatibility.
//
// Example Taskfile usage:
//
//	tasks:
//	  clean:
//	    cmds:
//	      - xplat rm -rf dist/    # Works on Windows, macOS, Linux
//	      - xplat mkdir -p build/ # Works on Windows, macOS, Linux
package osutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/otiai10/copy"
)

// Mkdir creates a directory. If parents is true, creates parent directories
// as needed and doesn't error if the directory already exists.
func Mkdir(path string, parents bool) error {
	if parents {
		return os.MkdirAll(path, 0755)
	}
	err := os.Mkdir(path, 0755)
	if err != nil && os.IsExist(err) && parents {
		return nil
	}
	return err
}

// Remove removes a file or directory. If recursive is true, removes
// directories and their contents. If force is true, ignores nonexistent files.
func Remove(path string, recursive, force bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) && force {
			return nil
		}
		return err
	}

	if info.IsDir() && !recursive {
		return fmt.Errorf("%s: is a directory (use recursive to remove)", path)
	}

	return os.RemoveAll(path)
}

// Copy copies a file or directory from src to dst. If recursive is true,
// copies directories and their contents.
func Copy(src, dst string, recursive bool) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() && !recursive {
		return fmt.Errorf("%s: is a directory (use recursive to copy)", src)
	}

	opts := copy.Options{
		OnSymlink: func(src string) copy.SymlinkAction {
			return copy.Shallow
		},
		PermissionControl: copy.PerservePermission,
		OnDirExists: func(src, dst string) copy.DirExistsAction {
			return copy.Merge
		},
	}

	return copy.Copy(src, dst, opts)
}

// Move moves or renames a file or directory from src to dst.
// If dst is an existing directory, moves src into it.
func Move(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// If destination is a directory, move into it
	dstInfo, err := os.Stat(dst)
	if err == nil && dstInfo.IsDir() {
		dst = filepath.Join(dst, srcInfo.Name())
	}

	return os.Rename(src, dst)
}

// Touch creates an empty file or updates the modification time of an existing file.
func Touch(path string) error {
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		if os.IsNotExist(err) {
			f, createErr := os.Create(path)
			if createErr != nil {
				return createErr
			}
			return f.Close()
		}
		return err
	}
	return nil
}

// Cat reads and returns the contents of a file.
func Cat(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// CatToWriter writes the contents of a file to a writer.
func CatToWriter(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	return err
}

// Exists returns true if the path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir returns true if the path is a directory.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsFile returns true if the path is a regular file.
func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
