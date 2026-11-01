package app

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type memAlertStore struct{ items []models.NetworkAlert }

func (m *memAlertStore) Read() ([]models.NetworkAlert, error) { return m.items, nil }
func (m *memAlertStore) Write(v []models.NetworkAlert) error  { m.items = v; return nil }

// An alert reaches the log even when no connector exists, and says so: the web UI is then the only place it shows.
func TestAlertNotifier_LogsTheAlertWhenNoConnectorIsSetUp(t *testing.T) {
	store := &memAlertStore{}
	s := &AppServices{}
	err := s.alertNotifier(netaudit.NewAlertLog(store, time.Now, nil), models.NetworkAlertPresence)(context.Background(), "Network audit: 1 new device.")
	if !errors.Is(err, netaudit.ErrNoConnector) {
		t.Fatalf("the alert must stay pending, got %v", err)
	}
	if len(store.items) != 1 || store.items[0].Delivery != models.NetworkAlertNoConnector || store.items[0].Kind != models.NetworkAlertPresence {
		t.Fatalf("got %+v", store.items)
	}
}

type fakeNeighbors struct {
	gw    netip.Addr
	err   error
	reads int
}

func (f *fakeNeighbors) Neighbors(context.Context) (netaudit.NeighborTable, error) {
	f.reads++
	return netaudit.NeighborTable{Gateway: f.gw}, f.err
}

func TestCachedGateway_ReadsTheRoutingTableOncePerTTLAndNeverTrustsAFailure(t *testing.T) {
	src := &fakeNeighbors{gw: netip.MustParseAddr("192.0.2.1")}
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	gw := cachedGateway(src, 30*time.Second, func() time.Time { return now })
	for i := 0; i < 50; i++ {
		if got := gw(); got != src.gw {
			t.Fatalf("gateway = %v", got)
		}
	}
	if src.reads != 1 {
		t.Errorf("reads = %d: a burst of datagrams must not read the routing table each", src.reads)
	}
	now = now.Add(31 * time.Second)
	src.err = errors.New("no table")
	if got := gw(); got.IsValid() {
		t.Errorf("a failed read must leave no gateway, got %v", got)
	}
	if src.reads != 2 {
		t.Errorf("reads = %d, want 2 after the cache expired", src.reads)
	}
}
