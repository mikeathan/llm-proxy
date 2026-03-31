package network

import (
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
