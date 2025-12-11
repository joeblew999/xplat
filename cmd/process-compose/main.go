// Process-Compose wrapper
//
// This is a thin wrapper around the process-compose library.
// We wrap it so we can use xplat binary:install for consistent
// cross-platform installation across all project tools.
//
// Upstream: https://github.com/F1bonacc1/process-compose
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/f1bonacc1/process-compose/src/cmd"
)

// version is set via ldflags at build time
var version = "dev"

func main() {
	// Check for -version flag before passing to process-compose
	// This ensures compatibility with our standard release:test task
	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Printf("process-compose %s\n", version)
		os.Exit(0)
	}

	// Also support -v shorthand
	ver := flag.Bool("v", false, "")
	flag.Parse()
	if *ver {
		fmt.Printf("process-compose %s\n", version)
		os.Exit(0)
	}

	// Reset args for process-compose (remove any parsed flags)
	os.Args = append([]string{os.Args[0]}, flag.Args()...)

	cmd.Execute()
}
