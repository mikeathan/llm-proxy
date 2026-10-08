package network

import (
	"errors"
	"net"
	"testing"
)

func TestResolveOrigin(t *testing.T) {
	tests := []struct {
		name     string
		bindAddr string
		want     string // partial match for dynamic IP
		exact    string
	}{
		{
			name:     "SpecificIpBind",
			bindAddr: "192.168.1.5:4000",
			exact:    "http://192.168.1.5:4000",
		},
		{
			name:     "LocalhostBind",
			bindAddr: "127.0.0.1:3000",
			exact:    "http://127.0.0.1:3000",
		},
		{
			name:     "GenericBind",
			bindAddr: ":8080",
			// We can't check exact because IP changes, but we check format
			want: "http://",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOrigin(tt.bindAddr)
			if tt.exact != "" {
				if got != tt.exact {
					t.Errorf("ResolveOrigin() = %v, want %v", got, tt.exact)
				}
			} else {
				if len(got) < len(tt.want) || got[:len(tt.want)] != tt.want {
					t.Errorf("ResolveOrigin() = %v, want prefix %v", got, tt.want)
				}
			}
		})
	}
}

func TestCheckAlwaysBlocked(t *testing.T) {
	for _, tc := range []struct {
		ip      string
		blocked bool
	}{
		{"0.0.0.0", true},
		{"::", true},
		{"127.0.0.1", true},
		{"127.1.2.3", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true}, // v4-in-v6 loopback
		{"169.254.169.254", true},  // cloud metadata
		{"fe80::1", true},
		{"224.0.0.1", true},
		{"ff02::1", true},
		{"ff01::1", true},
		{"192.168.1.1", false}, // LAN is policy, not always-blocked
		{"100.64.0.1", false},
		{"8.8.8.8", false},
		{"2606:4700:4700::1111", false},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			err := CheckAlwaysBlocked(net.ParseIP(tc.ip))
			if tc.blocked != (err != nil) {
				t.Fatalf("CheckAlwaysBlocked(%s) = %v, blocked want %v", tc.ip, err, tc.blocked)
			}
			if err != nil && !errors.Is(err, ErrAlwaysBlockedAddress) {
				t.Errorf("error %v does not wrap ErrAlwaysBlockedAddress", err)
			}
		})
	}
}

func TestIsLAN(t *testing.T) {
	for _, tc := range []struct {
		ip  string
		lan bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"fd00::1", true},
		{"100.64.0.1", true}, // CGNAT / Tailscale tailnet
		{"100.127.255.254", true},
		{"100.128.0.1", false},
		{"100.63.255.255", false},
		{"8.8.8.8", false},
	} {
		if got := IsLAN(net.ParseIP(tc.ip)); got != tc.lan {
			t.Errorf("IsLAN(%s) = %v, want %v", tc.ip, got, tc.lan)
		}
	}
}
