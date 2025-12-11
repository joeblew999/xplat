package env

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// GetLocalIP returns the non-loopback local IPv4 address for LAN access
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, address := range addrs {
		// Check the address type and if it is not a loopback
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			// Get IPv4 address
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no non-loopback IPv4 address found")
}

// GetLocalIPOrFallback returns the local IP address, or an empty string if not available.
// This is a convenience wrapper for cases where LAN access is optional (e.g., URL display).
// Use this when you want graceful degradation instead of error handling.
func GetLocalIPOrFallback() string {
	ip, _ := GetLocalIP()
	return ip // Returns empty string if error
}

// KillProcessOnPort kills any process listening on the specified port
// Uses lsof on Unix-like systems, netstat on Windows
// This is a fallback cleanup utility for processes without tracked handles
func KillProcessOnPort(port int) error {
	if runtime.GOOS == "windows" {
		return killProcessOnPortWindows(port)
	}
	return killProcessOnPortUnix(port)
}

// killProcessOnPortUnix kills process on port using lsof (macOS/Linux)
func killProcessOnPortUnix(port int) error {
	// Find process ID using lsof
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil {
		// lsof returns error if no process found - this is OK
		return nil
	}

	// Parse PIDs (lsof can return multiple)
	pidsStr := strings.TrimSpace(string(output))
	if pidsStr == "" {
		return nil
	}

	pids := strings.Split(pidsStr, "\n")
	for _, pidStr := range pids {
		pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
		if err != nil {
			continue
		}

		// Kill the process
		killCmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
		if err := killCmd.Run(); err != nil {
			fmt.Printf("   Warning: Failed to kill PID %d: %v\n", pid, err)
		} else {
			fmt.Printf("   Killed process %d on port %d\n", pid, port)
		}
	}

	return nil
}

// killProcessOnPortWindows kills process on port using netstat (Windows)
func killProcessOnPortWindows(port int) error {
	// Find process ID using netstat
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run netstat: %w", err)
	}

	// Parse netstat output to find PID
	lines := strings.Split(string(output), "\n")
	portStr := fmt.Sprintf(":%d", port)

	for _, line := range lines {
		if !strings.Contains(line, portStr) || !strings.Contains(line, "LISTENING") {
			continue
		}

		// Extract PID (last column in netstat output)
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		pidStr := fields[len(fields)-1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Kill the process using taskkill
		killCmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
		if err := killCmd.Run(); err != nil {
			fmt.Printf("   Warning: Failed to kill PID %d: %v\n", pid, err)
		} else {
			fmt.Printf("   Killed process %d on port %d\n", pid, port)
		}
	}

	return nil
}
