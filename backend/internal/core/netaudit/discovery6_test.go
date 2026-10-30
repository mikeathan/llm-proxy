package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"

	"llm-proxy/models"
)

type fakeDiscoverer struct {
	calls [][]string
	err   error
}

func (f *fakeDiscoverer) Discover(_ context.Context, interfaces []string) error {
	f.calls = append(f.calls, interfaces)
	return f.err
}

// sequenceSource answers each read with the next table, and the last one for every read after.
type sequenceSource struct {
	tables []NeighborTable
	reads  int
}

func (s *sequenceSource) Neighbors(context.Context) (NeighborTable, error) {
	t := s.tables[min(s.reads, len(s.tables)-1)]
	s.reads++
	return t, nil
}

func lanOnly(a netip.Addr) bool { return a.Is4() && a.As4()[0] == 192 }

func discoveryRig(enabled bool, d *fakeDiscoverer, tables ...NeighborTable) (*Inventory, *memInventory, *sequenceSource) {
	st := &memInventory{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{{MAC: macRouter, IP: "192.168.1.1", Approved: true}}}}
	src := &sequenceSource{tables: tables}
	v := newInventory(src, st)
	v.inScope = lanOnly
	v.WithDiscovery6(d, func() bool { return enabled })
	return v, st, src
}

func TestInventory_DiscoveryAsksOnlyTheLinksThatHoldInScopeDevices(t *testing.T) {
	d := &fakeDiscoverer{}
	before := NeighborTable{Entries: []Neighbor{onLink("192.168.1.1", macRouter, "wlan0"), onLink("172.17.0.2", macOther, "docker0")}}
	after := before
	after.Entries6 = []Neighbor{onLink("fe80::1", macTV, "wlan0")}
	v, st, src := discoveryRig(true, d, before, after)
	if _, err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(d.calls) != 1 || !slices.Equal(d.calls[0], []string{"wlan0"}) {
		t.Fatalf("discovery calls = %v, want one for wlan0 only (docker0 holds no in-scope device)", d.calls)
	}
	if src.reads != 2 {
		t.Errorf("the table is read again after the echo, reads = %d", src.reads)
	}
	if got := find(st.inv, macTV); got == nil || len(got.IPv6) != 1 {
		t.Errorf("the device the echo found is in the inventory: %+v", st.inv.Devices)
	}
	if v.Discovery6Problem() != "" {
		t.Errorf("problem = %q", v.Discovery6Problem())
	}
}

func TestInventory_DiscoveryIsOffUnlessSwitchedOn(t *testing.T) {
	d := &fakeDiscoverer{}
	v, _, src := discoveryRig(false, d, NeighborTable{Entries: []Neighbor{onLink("192.168.1.1", macRouter, "wlan0")}})
	if _, err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(d.calls) != 0 || src.reads != 1 {
		t.Errorf("off means no echo and one read: calls %v, reads %d", d.calls, src.reads)
	}
}

func TestInventory_ARefusedEchoIsAProblemTheScreenShowsAndTheLookStillCounts(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrDiscoveryNotPermitted, Discovery6NotPermitted},
		{ErrDiscoveryUnavailable, Discovery6Unavailable},
		{errors.New("socket exploded"), Discovery6Failed},
	}
	for _, c := range cases {
		d := &fakeDiscoverer{err: c.err}
		v, st, _ := discoveryRig(true, d, NeighborTable{Entries: []Neighbor{onLink("192.168.1.1", macRouter, "wlan0"), onLink("192.168.1.20", macTV, "wlan0")}})
		if _, err := v.Refresh(context.Background()); err != nil {
			t.Fatalf("%v: a refused echo must not fail the look: %v", c.err, err)
		}
		if v.Discovery6Problem() != c.want {
			t.Errorf("%v: problem = %q, want %q", c.err, v.Discovery6Problem(), c.want)
		}
		if find(st.inv, macTV) == nil {
			t.Errorf("%v: the IPv4 look is still folded in", c.err)
		}
		if st.inv.Baselined6 {
			t.Errorf("%v: a discovery that did not run is not a baseline", c.err)
		}
		d.err = nil
		if _, err := v.Refresh(context.Background()); err != nil || v.Discovery6Problem() != "" {
			t.Errorf("%v: the problem clears once it works: %q %v", c.err, v.Discovery6Problem(), err)
		}
	}
}

func TestInventory_SwitchingDiscoveryOffClearsItsProblem(t *testing.T) {
	d := &fakeDiscoverer{err: ErrDiscoveryNotPermitted}
	enabled := true
	v, _, _ := discoveryRig(true, d, NeighborTable{Entries: []Neighbor{onLink("192.168.1.1", macRouter, "wlan0")}})
	v.WithDiscovery6(d, func() bool { return enabled })
	_, _ = v.Refresh(context.Background())
	enabled = false
	_, _ = v.Refresh(context.Background())
	if v.Discovery6Problem() != "" {
		t.Errorf("an old problem would blame a feature that is off: %q", v.Discovery6Problem())
	}
}
