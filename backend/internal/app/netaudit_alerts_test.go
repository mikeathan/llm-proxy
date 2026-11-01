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

func TestCachedDevices_ReadsTheListOncePerTTLAndAnswersKnownAndNamed(t *testing.T) {
	reads := 0
	inv := models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:dd:ee:01", Name: "Laptop"}, {MAC: "aa:bb:cc:dd:ee:02"}}}
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	device := cachedDevices(func() (models.DeviceInventory, error) { reads++; return inv, nil }, 5*time.Second, func() time.Time { return now })
	for i := 0; i < 100; i++ {
		known, named, err := device("aa:bb:cc:dd:ee:01")
		if err != nil || !known || !named {
			t.Fatalf("named device = %v %v %v", known, named, err)
		}
	}
	if known, named, _ := device("aa:bb:cc:dd:ee:02"); !known || named {
		t.Errorf("an unnamed device is known and not named, got %v %v", known, named)
	}
	if known, _, _ := device("aa:bb:cc:dd:ee:03"); known {
		t.Error("an address not in the list is unknown")
	}
	if reads != 1 {
		t.Errorf("reads = %d: a stream of lookups must not read the list each time", reads)
	}
	now = now.Add(6 * time.Second)
	inv.Devices = append(inv.Devices, models.NetworkDevice{MAC: "aa:bb:cc:dd:ee:03"})
	if known, _, _ := device("aa:bb:cc:dd:ee:03"); !known || reads != 2 {
		t.Errorf("after the TTL the list is read again: known %v, reads %d", known, reads)
	}
}

func TestCachedDevices_AFailedReadIsAnErrorAndIsRetriedNotRemembered(t *testing.T) {
	fail := true
	device := cachedDevices(func() (models.DeviceInventory, error) {
		if fail {
			return models.DeviceInventory{}, errors.New("disk gone")
		}
		return models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:dd:ee:01"}}}, nil
	}, time.Hour, time.Now)
	if _, _, err := device("aa:bb:cc:dd:ee:01"); err == nil {
		t.Fatal("expected the read error")
	}
	fail = false
	if known, _, err := device("aa:bb:cc:dd:ee:01"); err != nil || !known {
		t.Errorf("after the disk came back: %v %v", known, err)
	}
}
