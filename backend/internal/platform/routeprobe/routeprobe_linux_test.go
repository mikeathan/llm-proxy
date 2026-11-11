//go:build linux

package routeprobe

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

// A probe to this computer's own loopback address reaches it at once: the closed port answers. This proves the socket, the error queue and the
// parser together, unprivileged, and sends nothing off this computer.
func TestAProbeToLoopbackIsAnsweredByTheAddressItself(t *testing.T) {
	r, err := Probe(context.Background(), Request{Dst: netip.MustParseAddr("127.0.0.1"), TTL: 1, Port: 33434, Wait: 2 * time.Second})
	if err != nil || r.Kind != Reached || r.From != netip.MustParseAddr("127.0.0.1") || r.RTT <= 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestAStoppedProbeEndsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Probe(ctx, Request{Dst: netip.MustParseAddr("127.0.0.1"), TTL: 1, Port: 1, Wait: time.Second}); err == nil {
		t.Fatal("a stopped probe answered")
	}
}
