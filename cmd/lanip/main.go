// lanip prints the local LAN IP address.
// Cross-platform (macOS, Linux, Windows).
//
// Usage: go run ./cmd/lanip
package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	ip := getLocalIP()
	if ip == "" {
		fmt.Fprintln(os.Stderr, "Could not determine LAN IP")
		os.Exit(1)
	}
	fmt.Println(ip)
}

// getLocalIP returns the preferred outbound IP of this machine.
// Uses UDP dial to 8.8.8.8 (doesn't actually send traffic).
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
