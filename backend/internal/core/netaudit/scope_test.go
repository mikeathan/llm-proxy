package netaudit

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"testing"

	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

type memStore struct{ cfg models.NetauditConfig }

func (m *memStore) Get() models.NetauditConfig { return m.cfg }
func (m *memStore) Update(fn func(*models.NetauditConfig) error) error {
	next := m.cfg
	if err := fn(&next); err != nil {
		return err
	}
	m.cfg = next
	return nil
}

type noNames struct{}

func (noNames) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return nil, errors.New("no resolver in this test")
}

func TestScope_SetRangesStoresCanonicalCIDRs(t *testing.T) {
	store := &memStore{}
	s := NewScope(store, noNames{}, nil)

	view, err := s.SetRanges(context.Background(), []string{"192.168.1.9/24", "10.0.0.5", "192.168.1.0/24"})
	if err != nil {
		t.Fatalf("SetRanges: %v", err)
	}
	want := []string{"10.0.0.5/32", "192.168.1.0/24"}
	if !reflect.DeepEqual(store.cfg.ScanRanges, want) {
		t.Errorf("stored = %v, want %v (canonical, sorted, deduplicated)", store.cfg.ScanRanges, want)
	}
	if len(view.Ranges) != 2 || view.Ranges[1].Devices != 254 {
		t.Errorf("view = %+v", view)
	}
}

func TestScope_ABadEntryRefusesTheWholeSave(t *testing.T) {
	store := &memStore{cfg: models.NetauditConfig{ScanRanges: []string{"10.0.0.0/24"}}}
	s := NewScope(store, noNames{}, nil)

	if _, err := s.SetRanges(context.Background(), []string{"192.168.1.0/24", "8.8.8.0/24"}); !errors.Is(err, scanscope.ErrNotScannable) {
		t.Fatalf("a public range must be refused, got %v", err)
	}
	if !reflect.DeepEqual(store.cfg.ScanRanges, []string{"10.0.0.0/24"}) {
		t.Errorf("a refused save must leave the scope unchanged, got %v", store.cfg.ScanRanges)
	}
}

func TestScope_PolicyFollowsTheStoredRanges(t *testing.T) {
	store := &memStore{}
	s := NewScope(store, noNames{}, nil)
	if _, err := s.Policy().Authorize(context.Background(), nil, []string{"192.168.1.5"}); !errors.Is(err, scanscope.ErrScanningOff) {
		t.Fatalf("an empty scope is off, got %v", err)
	}
	if _, err := s.SetRanges(context.Background(), []string{"192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Policy().Authorize(context.Background(), nil, []string{"192.168.1.5"}); err != nil || len(got) != 1 {
		t.Errorf("an address in the range must be authorised: %v, %v", got, err)
	}
	if _, err := s.Policy().Authorize(context.Background(), nil, []string{"192.168.2.5"}); !errors.Is(err, scanscope.ErrOutOfScope) {
		t.Errorf("an address outside must be refused, got %v", err)
	}
}

func TestScope_SetAllowUnconfined(t *testing.T) {
	store := &memStore{cfg: models.NetauditConfig{ScanRanges: []string{"10.0.0.0/24"}}}
	s := NewScope(store, noNames{}, nil)
	if err := s.SetAllowUnconfined(true); err != nil {
		t.Fatal(err)
	}
	if !store.cfg.AllowUnconfined || len(store.cfg.ScanRanges) != 1 {
		t.Errorf("only the flag may change: %+v", store.cfg)
	}
}

func TestScope_SuggestionsListOnlyScannablePrivateNetworks(t *testing.T) {
	cidr := func(s string) net.Addr {
		ip, n, _ := net.ParseCIDR(s)
		n.IP = ip
		return n
	}
	s := NewScope(&memStore{}, noNames{}, func() (map[string][]net.Addr, error) {
		return map[string][]net.Addr{
			"en0":  {cidr("192.168.1.23/24"), cidr("fe80::1/64")},
			"lo0":  {cidr("127.0.0.1/8")},
			"utun": {cidr("100.100.1.2/10")},
			"eth1": {cidr("10.1.2.3/8")},
			"wan":  {cidr("203.0.113.9/24")},
		}, nil
	})
	got, err := s.Suggestions()
	if err != nil {
		t.Fatal(err)
	}
	want := []Suggestion{
		{Interface: "en0", CIDR: "192.168.1.0/24", Devices: 254},
		{Interface: "eth1", CIDR: "10.1.2.0/24", Devices: 254},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("suggestions = %+v, want %+v", got, want)
	}
}
