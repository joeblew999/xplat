package syncgh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPollState(t *testing.T) {
	// Create temp dir for test
	tmpDir := t.TempDir()
	os.Setenv("XPLAT_HOME", tmpDir)
	defer os.Unsetenv("XPLAT_HOME")

	// Test 1: Load empty state (file doesn't exist)
	state, err := LoadPollState()
	if err != nil {
		t.Fatalf("LoadPollState failed: %v", err)
	}
	if len(state.Repos) != 0 {
		t.Errorf("Expected empty repos, got %d", len(state.Repos))
	}

	// Test 2: Set and check repo hash
	state.SetRepoHash("owner/repo", "main", "abc12345")

	hash := state.GetRepoHash("owner/repo", "main")
	if hash != "abc12345" {
		t.Errorf("Expected abc12345, got %s", hash)
	}

	// Test 3: HasChanged detection
	if !state.HasChanged("owner/repo", "main", "def67890") {
		t.Error("Expected HasChanged=true for different hash")
	}
	if state.HasChanged("owner/repo", "main", "abc12345") {
		t.Error("Expected HasChanged=false for same hash")
	}

	// Test 4: Save and reload
	if err := SavePollState(state); err != nil {
		t.Fatalf("SavePollState failed: %v", err)
	}

	state2, err := LoadPollState()
	if err != nil {
		t.Fatalf("LoadPollState after save failed: %v", err)
	}

	hash2 := state2.GetRepoHash("owner/repo", "main")
	if hash2 != "abc12345" {
		t.Errorf("After reload: expected abc12345, got %s", hash2)
	}

	t.Logf("✓ State persisted to %s/cache/syncgh-poll-state.json", tmpDir)
}

func TestTaskCacheInvalidation(t *testing.T) {
	// Create temp dir for project
	tmpDir := t.TempDir()

	// Create fake Task cache
	cacheDir := filepath.Join(tmpDir, ".task", "remote")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fake cached taskfile
	fakeFile := filepath.Join(cacheDir, "abc123.yml")
	if err := os.WriteFile(fakeFile, []byte("fake taskfile"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify cache exists
	if _, err := os.Stat(fakeFile); os.IsNotExist(err) {
		t.Fatal("Fake cache file should exist")
	}

	// Invalidate cache
	if err := InvalidateTaskCache(tmpDir, true); err != nil {
		t.Fatalf("InvalidateTaskCache failed: %v", err)
	}

	// Verify cache is gone
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("Cache directory should be deleted")
	}

	t.Log("✓ Task cache invalidated successfully")
}

func TestTaskCacheInvalidatorCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fake Task cache
	cacheDir := filepath.Join(tmpDir, ".task", "remote")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "test.yml"), []byte("test"), 0644)

	// Get the callback
	callback := TaskCacheInvalidator(tmpDir)

	// Simulate a change detection
	callback("owner/repo", "main", "old123", "new456")

	// Verify cache is gone
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("Cache should be invalidated after callback")
	}

	t.Log("✓ TaskCacheInvalidator callback works")
}
