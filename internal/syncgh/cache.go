// Package syncgh provides GitHub sync operations without requiring the gh CLI.
package syncgh

import (
	"log"
	"os"
	"path/filepath"
)

// InvalidateTaskCache clears the Task remote taskfile cache.
// This is called when syncgh detects upstream changes.
//
// Cache location: .task/remote/ in the working directory.
//
// Options:
//   - workDir: The project directory containing .task/
//   - aggressive: If true, clears ALL cached taskfiles. If false, does nothing (future: surgical invalidation)
func InvalidateTaskCache(workDir string, aggressive bool) error {
	if !aggressive {
		// Future: implement surgical invalidation based on repo+ref
		// For now, only aggressive mode is supported
		return nil
	}

	cachePath := filepath.Join(workDir, ".task", "remote")

	// Check if cache exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		// No cache to invalidate
		return nil
	}

	log.Printf("syncgh: Invalidating Task cache at %s", cachePath)
	return os.RemoveAll(cachePath)
}

// TaskCacheInvalidator returns an OnChange callback that invalidates Task cache.
// Use this with StatefulPoller:
//
//	poller.OnChange(syncgh.TaskCacheInvalidator(workDir))
func TaskCacheInvalidator(workDir string) func(repo, ref, oldHash, newHash string) {
	return func(repo, ref, oldHash, newHash string) {
		log.Printf("syncgh: Detected change in %s@%s (%s -> %s)", repo, ref, oldHash, newHash)

		if err := InvalidateTaskCache(workDir, true); err != nil {
			log.Printf("syncgh: Failed to invalidate Task cache: %v", err)
		} else {
			log.Printf("syncgh: Task cache invalidated successfully")
		}
	}
}
