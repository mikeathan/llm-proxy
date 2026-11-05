package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

type phoneAlertStore struct{ state models.PhoneHome }

func (m *phoneAlertStore) Read() (models.PhoneHome, error) { return m.state, nil }
func (m *phoneAlertStore) Write(s models.PhoneHome) error  { m.state = s; return nil }

var phoneAlertDay0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

// phoneAlertRig has a device that was watched for its learning week and then asked for new names.
type phoneAlertRig struct {
	watch *phonehome.Watch
	sent  []string
	err   error
	on    bool
	now   time.Time
	a     *PhoneHomeAlerts
}

func newPhoneAlertRig(t *testing.T) *phoneAlertRig {
	t.Helper()
	w, err := phonehome.New(&phoneAlertStore{})
	if err != nil {
		t.Fatal(err)
	}
	r := &phoneAlertRig{watch: w, on: true, now: phoneAlertDay0.Add(phonehome.LearnFor + 24*time.Hour)}
	w.Observe("aa:bb:cc:00:00:01", routerlog.Query{Type: "A", Name: "ntp.example.net"}, phoneAlertDay0)
	r.a = NewPhoneHomeAlerts(PhoneHomeAlertDeps{
		Watch:     w,
		Enabled:   func() bool { return r.on },
		Inventory: func() (models.DeviceInventory, error) { return phInv(), nil },
		Notify:    func(_ context.Context, msg string) error { r.sent = append(r.sent, msg); return r.err },
		Now:       func() time.Time { return r.now },
	})
	return r
}

func (r *phoneAlertRig) ask(name string) {
	r.watch.Observe("aa:bb:cc:00:00:01", routerlog.Query{Type: "A", Name: name}, r.now)
}

func TestPhoneHomeAlerts_TellOnceOfANewNameInFixedWordsNamingTheDeviceByAddress(t *testing.T) {
	r := newPhoneAlertRig(t)
	r.ask("c2.example.org")
	r.a.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], "192.168.1.20") || !strings.Contains(r.sent[0], "c2.example.org") {
		t.Fatalf("sent = %q", r.sent)
	}
	if strings.Contains(r.sent[0], "Living room") {
		t.Fatalf("sent = %q: the operator's name for a device is not in an alert", r.sent[0])
	}
	r.a.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %q: the same names are told once", r.sent)
	}
}

func TestPhoneHomeAlerts_ALargerSetOfNewNamesIsToldAgain(t *testing.T) {
	r := newPhoneAlertRig(t)
	r.ask("c2.example.org")
	r.a.Tick(context.Background())
	r.ask("d3.example.org")
	r.a.Tick(context.Background())
	if len(r.sent) != 2 || !strings.Contains(r.sent[1], "d3.example.org") {
		t.Fatalf("sent = %q", r.sent)
	}
}

func TestPhoneHomeAlerts_AFailedSendIsTriedAgainAndASuccessIsRememberedAcrossARestart(t *testing.T) {
	r := newPhoneAlertRig(t)
	r.ask("c2.example.org")
	r.err = errors.New("no connector")
	r.a.Tick(context.Background())
	r.err = nil
	r.a.Tick(context.Background())
	if len(r.sent) != 2 {
		t.Fatalf("sent = %d times, want the failed send tried again", len(r.sent))
	}
	// The record of what was told is part of the stored names, so a restart does not tell it again.
	store := &phoneAlertStore{}
	if err := r.watch.Flush(); err != nil {
		t.Fatal(err)
	}
	snap := r.watch.Snapshot()
	store.state = snap
	again, _ := phonehome.New(store)
	a := NewPhoneHomeAlerts(PhoneHomeAlertDeps{Watch: again, Enabled: func() bool { return true }, Inventory: func() (models.DeviceInventory, error) { return phInv(), nil },
		Notify: func(context.Context, string) error { t.Fatal("told again after a restart"); return nil }, Now: func() time.Time { return r.now }})
	a.Tick(context.Background())
}

func TestPhoneHomeAlerts_NeverTellOfADeviceThatAvoidsTheRoutersDNSOrWhileSwitchedOff(t *testing.T) {
	r := newPhoneAlertRig(t)
	r.ask("c2.example.org")
	r.on = false
	r.a.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Fatalf("sent = %q while off", r.sent)
	}
	// A device that is online and silent is a finding for the nightly report, not a message.
	w, _ := phonehome.New(&phoneAlertStore{})
	w.Observe("aa:bb:cc:00:00:09", routerlog.Query{Name: "example.com"}, phoneAlertDay0)
	a := NewPhoneHomeAlerts(PhoneHomeAlertDeps{Watch: w, Enabled: func() bool { return true },
		Inventory: func() (models.DeviceInventory, error) {
			return models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.20", FirstSeen: phoneAlertDay0.Add(-72 * time.Hour), LastSeen: r.now}}}, nil
		},
		Notify: func(context.Context, string) error { t.Fatal("a silent device must not be told"); return nil }, Now: func() time.Time { return r.now }})
	a.Tick(context.Background())
}

func TestPhoneHomeAlerts_TellAtMostAFewDevicesAtATimeAndTheRestOnTheNextTick(t *testing.T) {
	w, _ := phonehome.New(&phoneAlertStore{})
	inv := models.DeviceInventory{}
	now := phoneAlertDay0.Add(phonehome.LearnFor + 24*time.Hour)
	for i := 0; i < maxPhoneHomeAlertsPerTick+3; i++ {
		mac := "aa:bb:cc:00:10:" + string("0123456789abcdef"[i]) + "0"
		w.Observe(mac, routerlog.Query{Name: "ntp.example.net"}, phoneAlertDay0)
		w.Observe(mac, routerlog.Query{Name: "c2.example.org"}, now)
		inv.Devices = append(inv.Devices, models.NetworkDevice{MAC: mac, IP: "192.168.1." + string(rune('1'+i))})
	}
	var sent int
	a := NewPhoneHomeAlerts(PhoneHomeAlertDeps{Watch: w, Enabled: func() bool { return true }, Inventory: func() (models.DeviceInventory, error) { return inv, nil },
		Notify: func(context.Context, string) error { sent++; return nil }, Now: func() time.Time { return now }})
	a.Tick(context.Background())
	if sent != maxPhoneHomeAlertsPerTick {
		t.Fatalf("sent = %d, want %d on the first tick", sent, maxPhoneHomeAlertsPerTick)
	}
	a.Tick(context.Background())
	if sent != maxPhoneHomeAlertsPerTick+3 {
		t.Fatalf("sent = %d, want the rest on the next tick", sent)
	}
}

func TestNetworkAlertPhoneHome_IsAKnownKind(t *testing.T) {
	if !models.NetworkAlertPhoneHome.Valid() {
		t.Fatal("phone_home must be a valid alert kind")
	}
}
