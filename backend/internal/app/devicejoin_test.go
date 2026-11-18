package app

import (
	"bytes"
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/stackport"
	"llm-proxy/models"
)

// airStack hears what a sensing stack hears: it asks the lan port under its own purpose, as Radio does, and tags the addresses it "heard" with
// the vault itself, as Radio's recorder does from a frame. Its report and its finding are made of its own tags.
type airStack struct {
	portsOnly
	host  *stacksdk.Host
	heard []string
}

func (a *airStack) tags(ctx context.Context) ([][]byte, error) {
	const purpose = "radio_device_e3" // a rotated epoch: the join must follow it
	if _, err := a.host.LAN.Devices(ctx, purpose); err != nil {
		return nil, err
	}
	var out [][]byte
	for _, s := range a.heard {
		hw, _ := net.ParseMAC(s)
		t, err := a.host.Vault.Tag(ctx, purpose, hw)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (a *airStack) Air(ctx context.Context) (stacksdk.AirReport, error) {
	tags, err := a.tags(ctx)
	rep := stacksdk.AirReport{Listened: true}
	for _, t := range tags {
		rep.Devices = append(rep.Devices, stacksdk.AirFact{Tag: t, Heard: time.Now(), Ring: "house", Kind: "phone"})
	}
	return rep, err
}

func (a *airStack) Findings(ctx context.Context) ([]stacksdk.StackFinding, error) {
	tags, err := a.tags(ctx)
	var out []stacksdk.StackFinding
	for _, t := range tags {
		out = append(out, stacksdk.StackFinding{ID: "radio_join_foreign_network", Device: t, Count: 1})
	}
	return out, err
}

// The tag round trip through the production adapters (ADR-0061): the real vault and lan port, the purposes remembered, the join and the stack
// host. A stack's tag of an inventory device comes back as that device and its address; a neighbour's tag comes back as nothing.
func TestDeviceJoin_TheTagRoundTripThroughTheRealAdapters(t *testing.T) {
	ctx := context.Background()
	key := bytes.Repeat([]byte{7}, 32)
	vault := stackport.Vault{Key: func() ([]byte, error) { return key, nil }}
	inv := models.DeviceInventory{Devices: []models.NetworkDevice{{MAC: "AA:BB:CC:00:00:10", IP: "192.168.1.10", LastSeen: time.Now()}}}
	inventory := func() (models.DeviceInventory, error) { return inv, nil }
	asked := &stackport.AskedPurposes{}
	host := stacksdk.Host{Log: nopLog{}, Vault: vault, LAN: stackport.LAN{Vault: vault, Inventory: inventory, Asked: asked}}
	m := baseManifest(stacksdk.PortLog, stacksdk.PortVault, stacksdk.PortLAN)
	m.ReportsAir = true
	m.Modules[0].Finds = []string{"radio_join_foreign_network"}
	st := &airStack{portsOnly: portsOnly{manifest: m}, host: &host, heard: []string{"aa:bb:cc:00:00:10", "de:ad:be:ef:00:01"}}
	h, err := newStackHost(st, host)
	if err != nil {
		t.Fatal(err)
	}
	join := newDeviceJoin(inventory, h, vault, asked)

	view := join.View(ctx)
	if view.State != models.AirListening || len(view.ByMAC) != 1 {
		t.Fatalf("only the inventory's device is joined: %+v", view)
	}
	if d, ok := view.ByMAC["aa:bb:cc:00:00:10"]; !ok || d.Ring != "house" {
		t.Errorf("the phone's air: %+v %v", d, ok)
	}
	got, err := h.Offered(ctx)
	if err != nil || len(got) != 1 || got[0].Subject != netip.MustParseAddr("192.168.1.10") {
		t.Errorf("the finding lands on the phone's address and the neighbour's is dropped: %+v %v", got, err)
	}
}
