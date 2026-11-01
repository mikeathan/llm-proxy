package outsidelookup

import (
	"net/netip"
	"testing"
)

func TestClassify(t *testing.T) {
	for addr, want := range map[string]Kind{
		"203.0.113.7":     Public,
		"90.212.129.56":   Public,
		"8.8.8.8":         Public,
		"100.63.255.255":  Public, // just below the shared range
		"100.128.0.1":     Public, // just above it
		"192.168.0.137":   Private,
		"10.1.2.3":        Private,
		"172.16.0.1":      Private,
		"100.64.0.1":      Shared,
		"100.127.255.254": Shared,
		"169.254.10.10":   LinkLocal,
		"127.0.0.1":       Unusable,
		"0.0.0.0":         Unusable,
		"224.0.0.1":       Unusable,
		"255.255.255.255": Unusable,
		"::1":             Unusable,
		"2001:db8::1":     Unusable, // IPv6: the router's UPnP gives IPv4 only
	} {
		if got := Classify(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s: got %v, want %v", addr, got, want)
		}
	}
	if Classify(netip.Addr{}) != Unusable {
		t.Error("no address is unusable")
	}
}
