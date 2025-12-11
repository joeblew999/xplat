// Minimal CGO binary for testing CGO=1 release workflow
// This tool REQUIRES CGO to build, testing that xplat correctly
// handles CGO=1 tools (each platform builds its own binaries).
package main

/*
#include <stdlib.h>
*/
import "C"
import "fmt"

// version is set at build time via ldflags (-X main.version=xxx)
var version = "dev"

func main() {
	// Use C to force CGO requirement
	_ = C.rand()
	fmt.Println(version)
}
