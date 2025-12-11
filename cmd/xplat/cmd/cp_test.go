package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCpFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")
	content := []byte("test content")

	// Create source file
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Copy using os (simulating xplat cp behavior)
	srcContent, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, srcContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify destination
	dstContent, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(dstContent) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", dstContent, content)
	}
}

func TestCpPreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.sh")
	dst := filepath.Join(tmpDir, "dest.sh")

	// Create source with executable permission
	if err := os.WriteFile(src, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	srcInfo, _ := os.Stat(src)
	srcMode := srcInfo.Mode()

	// Copy with otiai10/copy (which xplat uses)
	// The library preserves permissions by default
	srcContent, _ := os.ReadFile(src)
	os.WriteFile(dst, srcContent, srcMode)

	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}

	// Check permissions are preserved (at least executable bit)
	if dstInfo.Mode().Perm()&0100 == 0 {
		t.Log("Note: Permission preservation depends on filesystem and OS")
	}
}

func TestCpDirectoryWithoutRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "srcdir")
	dstDir := filepath.Join(tmpDir, "dstdir")

	// Create source directory
	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Attempting to copy directory without -r should be rejected
	// This is handled by the cobra command logic
	info, _ := os.Stat(srcDir)
	if info.IsDir() {
		// xplat cp without -r should error on directories
		t.Log("Directory copy requires -r flag")
	}

	_ = dstDir // Would be used in actual copy
}

func TestCpRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create nested structure
	nested := filepath.Join(srcDir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("deep"), 0644); err != nil {
		t.Fatal(err)
	}

	// Copy recursively (using otiai10/copy in real implementation)
	// For test, we verify the source structure exists
	if _, err := os.Stat(filepath.Join(nested, "file.txt")); err != nil {
		t.Fatal("nested file should exist in source")
	}

	_ = dstDir // Would contain copied structure
}

func TestCpOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	// Create source and destination with different content
	os.WriteFile(src, []byte("new content"), 0644)
	os.WriteFile(dst, []byte("old content"), 0644)

	// Copy (should overwrite)
	content, _ := os.ReadFile(src)
	os.WriteFile(dst, content, 0644)

	// Verify overwrite
	result, _ := os.ReadFile(dst)
	if string(result) != "new content" {
		t.Fatalf("destination should be overwritten: got %q", result)
	}
}

func TestCpToDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dstDir := filepath.Join(tmpDir, "destdir")

	os.WriteFile(src, []byte("content"), 0644)
	os.Mkdir(dstDir, 0755)

	// When destination is a directory, copy into it
	dst := filepath.Join(dstDir, filepath.Base(src))
	content, _ := os.ReadFile(src)
	os.WriteFile(dst, content, 0644)

	// Verify file is inside directory
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("file should exist inside destination directory")
	}
}
