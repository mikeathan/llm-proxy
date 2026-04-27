package network

import (
	"llm-proxy/models"
	"testing"
)

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{"Empty host", "", models.AddrLocalhost},
		{"Generic listener 0.0.0.0", "0.0.0.0", models.AddrLocalhost},
		{"Specific IP", "192.168.1.10", "192.168.1.10"},
		{"Localhost", "127.0.0.1", "127.0.0.1"},
		{"Hostname", "vertex.local", "vertex.local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveHost(tt.host); got != tt.want {
				t.Errorf("ResolveHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		name string
		host string
		port string
		want string
	}{
		{"Standard", "127.0.0.1", "4001", "127.0.0.1:4001"},
		{"IPv6", "::1", "4001", "[::1]:4001"},
		{"Hostname", "localhost", "8080", "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Join(tt.host, tt.port); got != tt.want {
				t.Errorf("Join() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinDefault(t *testing.T) {
	host := "127.0.0.1"
	want := "127.0.0.1:" + models.DefaultAppPort
	if got := JoinDefault(host); got != want {
		t.Errorf("JoinDefault() = %v, want %v", got, want)
	}
}

func TestFormatURL(t *testing.T) {
	tests := []struct {
		name string
		host string
		port interface{}
		want string
	}{
		{"String port", "localhost", "4001", "http://localhost:4001"},
		{"Int port", "127.0.0.1", 9002, "http://127.0.0.1:9002"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatURL(tt.host, tt.port); got != tt.want {
				t.Errorf("FormatURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatLocalURL(t *testing.T) {
	reachable := GetReachableHost("0.0.0.0")
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{"Resolve 0.0.0.0", "0.0.0.0", 4001, "http://" + reachable + ":4001"},
		{"Keep specific IP", "192.168.1.5", 9002, "http://192.168.1.5:9002"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatLocalURL(tt.host, tt.port); got != tt.want {
				t.Errorf("FormatLocalURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
