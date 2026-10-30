//go:build linux

package neighbors

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestDiscover6_AgainstARealKernel needs a second machine on the link, so it is skipped unless NEIGH_DISCOVER_IFACE names the
// interface and either NEIGH_DISCOVER_PEER_MAC names a device that must be in the table afterwards, or
// NEIGH_DISCOVER_DENIED=1 says the host forbids unprivileged ICMP (net.ipv4.ping_group_range) and the call must say so.
func TestDiscover6_AgainstARealKernel(t *testing.T) {
	iface := os.Getenv("NEIGH_DISCOVER_IFACE")
	if iface == "" {
		t.Skip("set NEIGH_DISCOVER_IFACE and NEIGH_DISCOVER_PEER_MAC (or NEIGH_DISCOVER_DENIED=1) to check against a real kernel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := Discover6(ctx, []string{iface})
	if os.Getenv("NEIGH_DISCOVER_DENIED") == "1" {
		if !errors.Is(err, ErrNotPermitted) {
			t.Fatalf("a host that forbids unprivileged ICMP must give ErrNotPermitted, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	entries, err := read6()
	if err != nil {
		t.Fatal(err)
	}
	want := os.Getenv("NEIGH_DISCOVER_PEER_MAC")
	for _, e := range entries {
		if e.MAC == want && e.Interface == iface {
			return
		}
	}
	t.Fatalf("peer %s not in the table after discovery: %+v", want, entries)
}

func TestDiscover6_AnInterfaceThatDoesNotExistIsSkipped(t *testing.T) {
	if err := Discover6(context.Background(), []string{"nosuchif0"}); err != nil {
		t.Errorf("got %v", err)
	}
}

func TestDiscover6_StopsWaitingWhenTheContextEnds(t *testing.T) {
	if os.Getenv("NEIGH_DISCOVER_IFACE") == "" {
		t.Skip("needs a real multicast interface")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Discover6(ctx, []string{os.Getenv("NEIGH_DISCOVER_IFACE")}); err == nil || errors.Is(err, ErrNotPermitted) {
		t.Errorf("a cancelled context ends the wait with an error, got %v", err)
	}
}
