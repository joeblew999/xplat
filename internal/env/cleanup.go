package env

import "fmt"

// KillAll stops all running services (Caddy, Hugo, Via GUI)
// This is useful for cleaning up stray processes during development
// Uses existing stop functions for graceful shutdown where possible
func KillAll() error {
	fmt.Println("Stopping all services...")

	// 1. Stop Caddy using the proper API
	fmt.Println("\n1. Stopping Caddy...")
	if err := StopCaddy(); err != nil {
		fmt.Printf("   Warning: %v\n", err)
	}

	// 2. Stop Hugo server
	fmt.Println("\n2. Stopping Hugo server...")
	result := StopHugoServer()
	if result.Error != nil {
		fmt.Printf("   Warning: %v\n", result.Error)
	} else {
		fmt.Printf("   %s\n", result.Output)
	}

	// 3. Kill any process on port 3000 (Via GUI)
	//    Via GUI has no process handle, so use port-based cleanup
	fmt.Println("\n3. Killing processes on port 3000 (Via GUI)...")
	if err := KillProcessOnPort(3000); err != nil {
		fmt.Printf("   Warning: %v\n", err)
	} else {
		fmt.Println("   ✓ Cleaned up port 3000")
	}

	// 4. Kill any process on port 1313 (Hugo - backup in case StopHugoServer didn't work)
	fmt.Println("\n4. Killing processes on port 1313 (Hugo backup)...")
	if err := KillProcessOnPort(1313); err != nil {
		fmt.Printf("   Warning: %v\n", err)
	} else {
		fmt.Println("   ✓ Cleaned up port 1313")
	}

	fmt.Println("\n✓ All services stopped")
	return nil
}
