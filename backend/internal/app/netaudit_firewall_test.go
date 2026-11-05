package app

import (
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/models"
)

func TestFirewallFacts_ReadsTheSwitchesAndTheOwnPort(t *testing.T) {
	src := firewallSource{
		listeners: func() ([]hostwatch.Listener, error) { return []hostwatch.Listener{{Proto: "tcp", Port: 22}}, nil },
		networks:  func() []string { return []string{"192.168.50.0/24"} },
		gateway:   func() netip.Addr { return netip.MustParseAddr("192.168.50.1") },
		bind:      func() string { return ":4001" },
		decoysOn:  func() bool { return true },
		routerOn:  func() bool { return false },
	}
	f, err := src.facts()
	if err != nil || f.AppPort != 4001 || !slices.Equal(f.Decoys, []int{2222, 4445, 8023}) || f.RouterLogPort != 0 {
		t.Fatalf("facts = %+v, %v", f, err)
	}
	src.bind = func() string { return "127.0.0.1:4001" }
	if f, _ := src.facts(); f.AppPort != 0 {
		t.Fatalf("a program on this computer only needs no rule: %+v", f)
	}
	src.listeners = func() ([]hostwatch.Listener, error) { return nil, errors.New("no /proc") }
	if _, err := src.facts(); err == nil {
		t.Fatal("without the listeners there is no plan; the fixed guide is shown instead")
	}
}

func TestDecoyAlertText_AnUnreadableDeviceListStillSendsThePlainAlert(t *testing.T) {
	text := decoyAlertText(func() (models.DeviceInventory, error) { return models.DeviceInventory{}, errors.New("disk") }, time.Now)
	if got := text([]string{"10.0.0.5"}); got != netaudit.DecoyMessage([]string{"10.0.0.5"}) {
		t.Fatalf("got %q", got)
	}
}
