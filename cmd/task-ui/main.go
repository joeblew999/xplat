// Task-UI wrapper
//
// This is a thin wrapper around the task-ui library.
// We wrap it so we can use xplat binary:install for consistent
// cross-platform installation across all project tools.
//
// Upstream: https://github.com/titpetric/task-ui
package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/titpetric/task-ui/server"
)

// version is set via ldflags at build time
var version = "dev"

var (
	//go:embed templates/*.tpl public_html/static/*
	files embed.FS
)

func start(ctx context.Context) error {
	svc, err := server.New(&files)
	if err != nil {
		return err
	}
	return svc.Start(ctx)
}

func main() {
	// Check for -version flag for compatibility with standard release:test task
	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("task-ui %s\n", version)
		os.Exit(0)
	}

	ctx := context.Background()
	if err := start(ctx); err != nil {
		fmt.Println("Got error:", err)
	}
	fmt.Println("Exiting")
}
