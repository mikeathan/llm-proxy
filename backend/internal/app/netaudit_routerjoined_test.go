package app

import (
	"context"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

type joinedSettings struct{}

func (joinedSettings) Get() (string, string, string) {
	return "192.168.1.1:22", "admin", "ssh-ed25519 AAAA"
}
func (joinedSettings) Set(string, string, string) error { return nil }

type joinedSecret struct{}

func (joinedSecret) Value() string    { return "key" }
func (joinedSecret) Set(string) error { return nil }
func (joinedSecret) Clear() error     { return nil }

type joinedConn struct{ raw []byte }

func (c joinedConn) ReadClients(context.Context) ([]byte, error) { return c.raw, nil }
func (joinedConn) Close() error                                  { return nil }

type joinedPolicy struct{ cfg models.NetauditConfig }

func (p *joinedPolicy) Get() models.NetauditConfig { return p.cfg }
func (p *joinedPolicy) Update(f func(*models.NetauditConfig) error) error {
	return f(&p.cfg)
}

// Presence reads the router's list with no Radio stack at all: the read goes through the router manager the core always builds, so a device the
// router lists is online even when Radio is off or not hosted, and the switch and the guardrails still decide whether the router is read.
func TestRouterJoined_KeepsADeviceOnlineWithNoRadioStack(t *testing.T) {
	dir := t.TempDir()
	const phone = "aa:bb:cc:00:00:01"
	seen := time.Now().Add(-2 * time.Hour)
	store := persistence.NewDeviceInventoryStore(dir)
	if err := store.Write(models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{{MAC: phone, IP: "192.168.1.20", LastSeen: seen}}}); err != nil {
		t.Fatal(err)
	}
	inventory := netaudit.NewInventory(store, nil, nil)
	list := []byte(`{"02:00:00:00:00:f0":{"5G":{"` + phone + `":{"ip":"192.168.1.20","rssi":"-60"}}}}`)
	dials := 0
	manager := routerconn.New(routerconn.Deps{Settings: joinedSettings{}, Secret: joinedSecret{},
		Dial: func(context.Context, routerssh.Target) (routerconn.ClientReader, error) {
			dials++
			return joinedConn{raw: list}, nil
		}})
	policy := &joinedPolicy{}
	lanOK := true
	config, joined := newRouterJoined(policy, manager, inventory, func() (bool, string) { return lanOK, "no LAN" })

	joined.Look(context.Background())
	if dials != 0 {
		t.Fatalf("switched off, the router was signed in to %d times", dials)
	}
	if err := config.Set(true); err != nil {
		t.Fatal(err)
	}
	lanOK = false
	joined.Look(context.Background())
	if dials != 0 {
		t.Fatalf("with the local network closed, the router was signed in to %d times", dials)
	}
	lanOK = true
	joined.Look(context.Background())
	inv, _ := inventory.View()
	d := inv.Devices[0]
	if dials != 1 || d.RouterSeen.IsZero() || !d.LastSeen.Equal(seen) {
		t.Fatalf("dials = %d, device = %+v; want one sign-in and the router's mark with LastSeen unchanged", dials, d)
	}
	st := netaudit.PresenceStatus{LastLook: time.Now()}
	if got := netaudit.PresenceOf(d, true, st, time.Now()); got != models.PresenceOnline {
		t.Errorf("presence = %q, want online from the router's list", got)
	}
}

// Before the Network services are wired, presence's look at the router does nothing.
func TestRouterJoined_NothingBeforeTheServicesAreWired(t *testing.T) {
	(&AppServices{}).routerJoined(context.Background())
}
