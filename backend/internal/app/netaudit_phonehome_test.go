package app

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

type modelsPhoneHome = models.PhoneHome

type seen struct {
	mac string
	q   routerlog.Query
}

type lookupRecorder struct{ got []seen }

func (r *lookupRecorder) Observe(mac string, q routerlog.Query, _ time.Time) {
	r.got = append(r.got, seen{mac, q})
}

func TestPhoneHomeIntake_KeepsALookupByTheSendersHardwareAddress(t *testing.T) {
	rec := &lookupRecorder{}
	in := phoneHomeIntake{
		enabled: func() bool { return true }, watch: rec,
		resolve: func(a netip.Addr) (string, bool) {
			return "aa:bb:cc:00:00:01", a == netip.MustParseAddr("192.168.1.20")
		},
		own: func(a netip.Addr) bool { return a == netip.MustParseAddr("192.168.1.5") },
	}
	q := routerlog.Query{Type: "A", Name: "example.com", From: netip.MustParseAddr("192.168.1.20")}
	in.Observe(q, time.Now())
	if len(rec.got) != 1 || rec.got[0].mac != "aa:bb:cc:00:00:01" {
		t.Fatalf("got = %+v", rec.got)
	}
	in.Observe(routerlog.Query{Name: "example.com", From: netip.MustParseAddr("192.168.1.99")}, time.Now())
	in.Observe(routerlog.Query{Name: "example.com", From: netip.MustParseAddr("192.168.1.5")}, time.Now())
	if len(rec.got) != 1 {
		t.Fatalf("got = %+v; a sender with no hardware address and this computer's own lookups are not kept", rec.got)
	}
}

func TestPhoneHomeIntake_KeepsNothingWhileTheWatchIsOff(t *testing.T) {
	rec := &lookupRecorder{}
	in := phoneHomeIntake{enabled: func() bool { return false }, watch: rec, resolve: func(netip.Addr) (string, bool) { return "aa:bb:cc:00:00:01", true }, own: func(netip.Addr) bool { return false }}
	in.Observe(routerlog.Query{Name: "example.com", From: netip.MustParseAddr("192.168.1.20")}, time.Now())
	if len(rec.got) != 0 {
		t.Fatalf("got = %+v while the watch is off", rec.got)
	}
}

type countingSource struct {
	reads int
	table netaudit.NeighborTable
}

func (c *countingSource) Neighbors(context.Context) (netaudit.NeighborTable, error) {
	c.reads++
	return c.table, nil
}

func TestCachedMACs_ReadTheTableOncePerTTLAndKnowIPv4AndIPv6(t *testing.T) {
	src := &countingSource{table: netaudit.NeighborTable{
		Entries:  []netaudit.Neighbor{{Addr: netip.MustParseAddr("192.168.1.20"), MAC: "aa:bb:cc:00:00:01"}},
		Entries6: []netaudit.Neighbor{{Addr: netip.MustParseAddr("fd00::20"), MAC: "aa:bb:cc:00:00:01"}},
	}}
	now := time.Now()
	resolve := cachedMACs(src, 10*time.Second, func() time.Time { return now })
	for _, a := range []string{"192.168.1.20", "fd00::20"} {
		if mac, ok := resolve(netip.MustParseAddr(a)); !ok || mac != "aa:bb:cc:00:00:01" {
			t.Errorf("%s = %q %v", a, mac, ok)
		}
	}
	if _, ok := resolve(netip.MustParseAddr("192.168.1.77")); ok {
		t.Error("an address the table does not hold must not resolve")
	}
	if src.reads != 1 {
		t.Fatalf("reads = %d, want one for a burst of lookups", src.reads)
	}
	now = now.Add(11 * time.Second)
	resolve(netip.MustParseAddr("192.168.1.20"))
	if src.reads != 2 {
		t.Fatalf("reads = %d, want the table read again after the ttl", src.reads)
	}
}

func TestPhoneHomeSignals_AreEmptyWhileTheWatchIsOff(t *testing.T) {
	w := phonehome.NewEmpty(nopStore{})
	on := false
	s := phoneHomeSignals{enabled: func() bool { return on }, watch: w}
	if got := s.Signals([]phonehome.InventoryDevice{{MAC: "aa:bb:cc:00:00:01", FirstSeen: time.Now().Add(-90 * time.Hour), LastSeen: time.Now()}}, time.Now()); got != nil {
		t.Fatalf("signals = %+v while off", got)
	}
}

type nopStore struct{}

func (nopStore) Read() (m modelsPhoneHome, err error) { return }
func (nopStore) Write(modelsPhoneHome) error          { return nil }
