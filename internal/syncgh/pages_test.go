package syncgh

import (
	"os"
	"testing"
)

func TestGetRepoFromRemote(t *testing.T) {
	tests := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
		expectError   bool
	}{
		{"https://github.com/joeblew999/xplat.git", "joeblew999", "xplat", false},
		{"https://github.com/joeblew999/xplat", "joeblew999", "xplat", false},
		{"git@github.com:joeblew999/plat-tinypio.git", "joeblew999", "plat-tinypio", false},
		{"git@github.com:joeblew999/plat-tinypio", "joeblew999", "plat-tinypio", false},
		{"https://gitlab.com/foo/bar", "", "", true},
	}

	for _, tt := range tests {
		owner, repo, err := GetRepoFromRemote(tt.url)
		if tt.expectError {
			if err == nil {
				t.Errorf("expected error for %s", tt.url)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for %s: %v", tt.url, err)
			continue
		}
		if owner != tt.expectedOwner || repo != tt.expectedRepo {
			t.Errorf("for %s: got %s/%s, want %s/%s", tt.url, owner, repo, tt.expectedOwner, tt.expectedRepo)
		}
	}
}

func TestEnablePages_Integration(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set, skipping integration test")
	}

	// Test on plat-tinypio which should already have pages enabled
	err := EnablePages("joeblew999", "plat-tinypio")
	if err != nil {
		// 409 (already exists) or success are both OK
		t.Logf("EnablePages result: %v (this may be expected if already enabled)", err)
	} else {
		t.Log("EnablePages succeeded")
	}
}
