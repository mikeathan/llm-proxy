package app

import (
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
)

// The decoys and Nearby both ask "is this address this computer's own?" for every connection or datagram. The answer must come
// from the addresses the scan scope reads (which fall back to the default route's address where the service may not open a
// netlink socket), read at most once per ttl, and a failed read must keep the last good answer.
func TestCachedOwnAddress_UsesTheScopesAddressesOncePerTTLAndKeepsThemThroughAFailure(t *testing.T) {
	reads := 0
	fail := false
	scope := netaudit.NewScope(&memPolicyStore{}, nil, func() (map[string][]net.Addr, error) {
		reads++
		if fail {
			return nil, errors.New("netlink refused")
		}
		return map[string][]net.Addr{"eth0": {&net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)}}}, nil
	})
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	own := cachedOwnAddress(scope, 10*time.Second, func() time.Time { return now })

	host, other := netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("192.168.1.66")
	if !own(host) || own(other) {
		t.Fatalf("own(host) = %v, own(other) = %v", own(host), own(other))
	}
	for range 100 {
		own(other)
	}
	if reads != 1 {
		t.Fatalf("the addresses were read %d times within the ttl, want 1", reads)
	}
	fail = true
	now = now.Add(11 * time.Second)
	if !own(host) {
		t.Fatal("a failed read forgot this computer's address, so its own connections would count as an intruder's")
	}
	if reads != 2 {
		t.Fatalf("reads = %d, want a second read after the ttl", reads)
	}
}
