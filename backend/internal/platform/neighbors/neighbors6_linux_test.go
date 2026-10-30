//go:build linux

package neighbors

import (
	"net/netip"
	"os"
	"testing"
)

// TestRead6_AgainstARealKernel reads the neighbour table from the kernel this test runs on. It is skipped unless
// NEIGH_EXPECT_ADDR names an address that was put in the table first (for example `ip -6 neigh add 2001:db8::5 lladdr
// 02:00:5e:00:53:99 dev d0 nud reachable`), because the table of an ordinary machine is whatever it happens to hold.
func TestRead6_AgainstARealKernel(t *testing.T) {
	want := os.Getenv("NEIGH_EXPECT_ADDR")
	if want == "" {
		t.Skip("set NEIGH_EXPECT_ADDR and NEIGH_EXPECT_MAC to check against a real kernel")
	}
	entries, err := read6()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Addr == netip.MustParseAddr(want) {
			if e.MAC != os.Getenv("NEIGH_EXPECT_MAC") || e.Interface == "" {
				t.Fatalf("entry = %+v, want MAC %s and an interface name", e, os.Getenv("NEIGH_EXPECT_MAC"))
			}
			return
		}
	}
	t.Fatalf("%s not in the kernel's table: %+v", want, entries)
}
