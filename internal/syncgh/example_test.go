package syncgh_test

import (
	"fmt"
	"os"
	"time"

	"github.com/joeblew999/xplat/internal/syncgh"
)

// Example_statefulPoller demonstrates using StatefulPoller with cache invalidation.
func Example_statefulPoller() {
	// Configure repos to watch
	repos := []syncgh.RepoConfig{
		{Subsystem: "joeblew999/xplat", Branch: "main"},
	}

	// Create stateful poller (tracks commit hashes between polls)
	poller, err := syncgh.NewStatefulPoller(5*time.Minute, repos, os.Getenv("GITHUB_TOKEN"))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Wire up Task cache invalidation
	workDir, _ := os.Getwd()
	poller.OnChange(syncgh.TaskCacheInvalidator(workDir))

	// Show that poller is configured with repos
	fmt.Printf("Configured to poll %d repo(s)\n", len(repos))

	// In real usage: poller.StartAsync() to run in background
	// For demo, we just show it's configured
	fmt.Println("Poller configured with cache invalidation")

	// Output:
	// Configured to poll 1 repo(s)
	// Poller configured with cache invalidation
}
