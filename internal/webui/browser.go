package web

import (
	"os/exec"
	"runtime"
)

// openBrowser opens the default browser to the given URL.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return nil // Unsupported platform
	}

	return cmd.Start()
}
