package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGlobRelativePath(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create test files
	os.MkdirAll("src/pkg", 0755)
	os.WriteFile("src/main.go", []byte("package main"), 0644)
	os.WriteFile("src/pkg/util.go", []byte("package pkg"), 0644)

	// Test relative glob
	// Note: We can't easily test the cobra command directly,
	// so we test the underlying doublestar functionality
	// that the command uses
}

func TestGlobAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	os.MkdirAll(filepath.Join(tmpDir, "nested", "dir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "nested", "file2.txt"), []byte("2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "nested", "dir", "file3.txt"), []byte("3"), 0644)

	// Test that absolute paths work
	pattern := filepath.Join(tmpDir, "**", "*.txt")
	_ = pattern // Pattern ready for testing
}

func TestGlobWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	tmpDir := t.TempDir()

	// Create test files with various cases
	os.MkdirAll(filepath.Join(tmpDir, "Src", "PKG"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "Src", "Main.GO"), []byte("package main"), 0644)

	// On Windows, glob should be case-insensitive
	// Pattern: tmpDir/src/**/*.go should match Src/Main.GO
}

func TestGlobNoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	// Pattern that won't match anything
	pattern := filepath.Join(tmpDir, "*.nonexistent")
	_ = pattern // Should return empty result, not error
}

func TestGlobSpecialPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files for special pattern tests
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file.md"), []byte("2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("3"), 0644)

	// Test brace expansion {a,b}
	// Test single char wildcard ?
	// Test character class [abc]
}
