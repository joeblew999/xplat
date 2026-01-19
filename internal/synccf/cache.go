// Package synccf provides Cloudflare sync operations without requiring the wrangler CLI.
package synccf

import (
	"context"
	"log"
	"os"
	"path/filepath"
)

// InvalidateTaskCache clears the Task remote taskfile cache.
// This is called when synccf receives events indicating upstream changes.
//
// Cache location: .task/remote/ in the working directory.
//
// Options:
//   - workDir: The project directory containing .task/
//   - aggressive: If true, clears ALL cached taskfiles. If false, does nothing (future: surgical invalidation)
func InvalidateTaskCache(workDir string, aggressive bool) error {
	if !aggressive {
		// Future: implement surgical invalidation based on event metadata
		// For now, only aggressive mode is supported
		return nil
	}

	cachePath := filepath.Join(workDir, ".task", "remote")

	// Check if cache exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		// No cache to invalidate
		return nil
	}

	log.Printf("synccf: Invalidating Task cache at %s", cachePath)
	return os.RemoveAll(cachePath)
}

// TaskCacheInvalidator returns an OnAny callback that invalidates Task cache on Pages deploy events.
// Use this with ReceiveHandler:
//
//	synccf.RunReceiveServer(port, synccf.ReceiveCallbacks{
//		OnPagesDeploy: synccf.TaskCacheInvalidator(workDir),
//	})
func TaskCacheInvalidator(workDir string) func(ctx context.Context, event WorkerEvent) error {
	return func(ctx context.Context, event WorkerEvent) error {
		log.Printf("synccf: Received %s event: %s on %s", event.Type, event.Action, event.Resource)

		// Only invalidate cache on Pages deploy events
		// This assumes remote taskfiles are hosted on Cloudflare Pages
		if event.Type != "pages_deploy" {
			return nil
		}

		if err := InvalidateTaskCache(workDir, true); err != nil {
			log.Printf("synccf: Failed to invalidate Task cache: %v", err)
			return err
		}

		log.Printf("synccf: Task cache invalidated successfully")
		return nil
	}
}

// AllEventsLoggerWithInvalidation returns a callback that logs all events
// and invalidates cache on Pages deploys.
func AllEventsLoggerWithInvalidation(workDir string) func(ctx context.Context, event WorkerEvent) error {
	invalidator := TaskCacheInvalidator(workDir)

	return func(ctx context.Context, event WorkerEvent) error {
		// Log the event
		log.Printf("EVENT: [%s] %s on %s (source: %s)", event.Type, event.Action, event.Resource, event.Source)

		// If it's a pages_deploy, also invalidate cache
		if event.Type == "pages_deploy" {
			return invalidator(ctx, event)
		}

		return nil
	}
}
