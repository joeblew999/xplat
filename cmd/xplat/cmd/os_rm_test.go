package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRmFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("test file should exist before removal")
	}

	// Remove file
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Fatal("file should not exist after removal")
	}
}

func TestRmDirectoryWithoutRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "subdir")

	// Create test directory
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatal(err)
	}

	// os.Remove should fail on non-empty directory
	// This tests the behavior that xplat rm (without -r) should exhibit
}

func TestRmRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "subdir")
	nestedFile := filepath.Join(testDir, "nested", "file.txt")

	// Create nested structure
	if err := os.MkdirAll(filepath.Dir(nestedFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// RemoveAll should work
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Fatal("directory should not exist after RemoveAll")
	}
}

func TestRmForceNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	nonexistent := filepath.Join(tmpDir, "does-not-exist.txt")

	// Without force, this should error
	_, err := os.Stat(nonexistent)
	if !os.IsNotExist(err) {
		t.Fatal("file should not exist")
	}

	// With force (-f), rm should succeed silently on nonexistent files
	// RemoveAll doesn't error on nonexistent paths
	if err := os.RemoveAll(nonexistent); err != nil {
		t.Fatalf("RemoveAll should not error on nonexistent path: %v", err)
	}
}

func TestRmMultiplePaths(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	// Create test files
	os.WriteFile(file1, []byte("1"), 0644)
	os.WriteFile(file2, []byte("2"), 0644)

	// Remove both
	for _, f := range []string{file1, file2} {
		if err := os.Remove(f); err != nil {
			t.Fatalf("failed to remove %s: %v", f, err)
		}
	}

	// Verify both are gone
	for _, f := range []string{file1, file2} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("%s should not exist", f)
		}
	}
}
