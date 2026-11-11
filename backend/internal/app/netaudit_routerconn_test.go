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
