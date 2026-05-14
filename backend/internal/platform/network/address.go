package network

import (
	"fmt"
	"llm-proxy/models"
	"net"
)

// ResolveHost returns the host as-is unless it's empty, in which case it defaults to localhost.
func ResolveHost(host string) string {
	if host == "" {
		return models.AddrLocalhost
	}
	return host
}

// GetReachableHost attempts to convert a listener address (like 0.0.0.0) into 
// an address that is reachable from the network (the primary LAN IP).
func GetReachableHost(host string) string {
	if host == "" || host == models.AddrLocalhost {
		return models.AddrLocalhost
	}
	if host == models.AddrAllInterfaces {
		if ip, err := GetPrimaryIP(); err == nil {
			return ip
		}
		return models.AddrLocalhost
	}
	return host
}

// GetPrimaryIP returns the first non-loopback IPv4 address found on the system.
func GetPrimaryIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no primary IP found")
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

// FormatLocalURL constructs a URL ensuring the host is reachable from the network.
func FormatLocalURL(host string, port interface{}) string {
	return FormatURL(GetReachableHost(host), port)
}
