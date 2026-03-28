package network

import (
	"fmt"
	"net"
)

// ResolveOrigin constructs the MCP client origin URL.
// It takes an optional override (from env var) and a bind address.
// If the bind address is generic (0.0.0.0) or empty, it resolves the outbound IP.
func ResolveOrigin(bindAddr string) string {

	if bindAddr == "" {
		bindAddr = ":8080"
	}

	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		// Fallback for malformed bind address
		return "http://localhost:8080"
	}

	// If host is specific (e.g. 192.168.1.50), use it
	if host != "" && host != "0.0.0.0" {
		return fmt.Sprintf("http://%s:%s", host, port)
	}

	// If generic, find outbound IP
	ip := getOutboundIP()
	if ip == "" {
		return fmt.Sprintf("http://localhost:%s", port)
	}

	return fmt.Sprintf("http://%s:%s", ip, port)
}

// getOutboundIP gets the preferred outbound ip of this machine
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
