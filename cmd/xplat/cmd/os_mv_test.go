package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMvRenameFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "old.txt")
	dst := filepath.Join(tmpDir, "new.txt")

	// Create source file
	if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rename
	if err := os.Rename(src, dst); err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	// Source should not exist
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should not exist after rename")
	}

	// Destination should exist with correct content
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "content" {
		t.Fatalf("content mismatch: got %q", content)
	}
}

func TestMvRenameDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "olddir")
	dst := filepath.Join(tmpDir, "newdir")

	// Create source directory with file
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rename directory
	if err := os.Rename(src, dst); err != nil {
		t.Fatalf("rename directory failed: %v", err)
	}

	// Source should not exist
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source directory should not exist")
	}

	// Destination should exist with file inside
	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
		t.Fatal("file should exist in renamed directory")
	}
}

func TestMvIntoDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "file.txt")
	dstDir := filepath.Join(tmpDir, "destdir")

	// Create source and destination directory
	os.WriteFile(src, []byte("content"), 0644)
	os.Mkdir(dstDir, 0755)

	// Move file into directory
	dst := filepath.Join(dstDir, filepath.Base(src))
	if err := os.Rename(src, dst); err != nil {
		t.Fatalf("move into directory failed: %v", err)
	}

	// Source should not exist
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should not exist after move")
	}

	// File should exist in destination directory
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("file should exist in destination directory")
	}
}

func TestMvOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	// Create source and destination
	os.WriteFile(src, []byte("new"), 0644)
	os.WriteFile(dst, []byte("old"), 0644)

	// Move (overwrites destination)
	if err := os.Rename(src, dst); err != nil {
		t.Fatalf("move/overwrite failed: %v", err)
	}

	// Verify content
	content, _ := os.ReadFile(dst)
	if string(content) != "new" {
		t.Fatalf("destination should have new content: got %q", content)
	}
}

func TestMvNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "nonexistent.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	// Should error on nonexistent source
	err := os.Rename(src, dst)
	if !os.IsNotExist(err) {
		t.Fatalf("should error on nonexistent source, got: %v", err)
	}
}
