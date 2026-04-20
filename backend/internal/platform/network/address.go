package network

import (
	"fmt"
	"llm-proxy/models"
	"net"
)

// ResolveHost converts a listener address (like 0.0.0.0) into a dialable address (like 127.0.0.1).
func ResolveHost(host string) string {
	if host == "" || host == models.AddrAllInterfaces {
		return models.AddrLocalhost
	}
	return host
}

// Join safely combines a host and port using net.JoinHostPort.
func Join(host, port string) string {
	return net.JoinHostPort(host, port)
}

// JoinDefault combines a host with the system's default application port.
func JoinDefault(host string) string {
	return Join(host, models.DefaultAppPort)
}

// FormatURL constructs a full HTTP URL from a host and port.
func FormatURL(host string, port interface{}) string {
	return fmt.Sprintf("http://%s:%v", host, port)
}

// FormatLocalURL constructs a URL ensuring the host is dialable (resolving 0.0.0.0).
func FormatLocalURL(host string, port interface{}) string {
	return FormatURL(ResolveHost(host), port)
}
