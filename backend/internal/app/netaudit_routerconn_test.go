package app

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/internal/core/netaudit/routerhistory"
	"llm-proxy/models"
)

type memHistoryStore struct{ state models.RouterHistory }

func (m *memHistoryStore) Read() (models.RouterHistory, error) { return m.state, nil }
func (m *memHistoryStore) Write(h models.RouterHistory) error  { m.state = h; return nil }

func readOf(ip string) *models.RouterConnections {
	return &models.RouterConnections{Devices: []models.RouterDevice{{Address: ip, Total: 1, Places: []models.RouterPlace{
		{Address: "1.1.1.1", Port: 443, Proto: "tcp", Count: 1, Owner: &models.TrafficOwner{ASN: 13335, Name: "Cloudflare", Country: "US"}},
	}}}}
}

func TestTheObserverKeepsNothingWhileTheWatchIsOff(t *testing.T) {
	store := &memHistoryStore{}
	h := routerhistory.NewEmpty(store)
	on := false
	observe := routerConnObserver(func() bool { return on }, h, nil, time.Now)
	observe(readOf("192.168.50.9"))
	if len(store.state.Devices) != 0 {
		t.Fatal("a watch that is off must keep no history")
	}
	on = true
	observe(readOf("192.168.50.9"))
	if _, ok := store.state.Devices["ip:192.168.50.9"]; !ok {
		t.Fatalf("a device the inventory does not know is kept by its address: %+v", store.state.Devices)
	}
}

func TestRefreshTreatsAnUnfinishedSetupAndAJustReadRouterAsNothingToDo(t *testing.T) {
	for _, err := range []error{routerconn.ErrNotSet, routerconn.ErrBusy} {
		if got := mapRefreshError(err); !errors.Is(got, netaudit.ErrRouterConnSkipped) {
			t.Fatalf("%v must read as skipped, got %v", err, got)
		}
	}
	real := errors.New("the router refused the key")
	if got := mapRefreshError(real); !errors.Is(got, real) {
		t.Fatalf("a real failure must stay a failure, got %v", got)
	}
	if mapRefreshError(nil) != nil {
		t.Fatal("success is success")
	}
}

func TestTheWatchCanBeSwitchedOnOnlyWhenTheSetupIsFinished(t *testing.T) {
	lanOK := func() (bool, string) { return true, "" }
	lanOff := func() (bool, string) { return false, "local network is off" }
	if ok, why := routerConnReady(lanOff, nil); ok || why != "local network is off" {
		t.Fatalf("the local network is needed first: %v %q", ok, why)
	}
	if ok, why := routerConnReady(lanOK, nil); ok || why == "" {
		t.Fatalf("no manager means not ready: %v %q", ok, why)
	}
}

func TestTheNightlyReadSendsNothingWhenTheGuardrailsHaveClosedTheLocalNetwork(t *testing.T) {
	w := routerConnWatch{ready: func() (bool, string) { return false, "Local network access is off" }}
	err := w.Refresh(t.Context())
	if err == nil || errors.Is(err, netaudit.ErrRouterConnSkipped) || err.Error() != "Local network access is off" {
		t.Fatalf("a closed guardrail is a failure the report says, got %v", err)
	}
}

func TestTheReportsReadsAreHeldToOnePerGapWhoeverStartsTheRuns(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	g := newReadGate(3*time.Hour, func() time.Time { return now })
	if !g.Take() {
		t.Fatal("the first read is allowed")
	}
	for i := 0; i < 50; i++ {
		now = now.Add(time.Minute)
		if g.Take() {
			t.Fatalf("a read %d minutes after the last must be refused", i+1)
		}
	}
	now = now.Add(3 * time.Hour)
	if !g.Take() {
		t.Fatal("after the gap a read is allowed again")
	}
}

func TestARefusedReadReadsAsNothingToDoNotAFailure(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	g := newReadGate(time.Hour, func() time.Time { return now })
	g.Take()
	w := routerConnWatch{gate: g}
	if err := w.Refresh(t.Context()); !errors.Is(err, netaudit.ErrRouterConnSkipped) {
		t.Fatalf("a read inside the gap must be skipped, got %v", err)
	}
}
