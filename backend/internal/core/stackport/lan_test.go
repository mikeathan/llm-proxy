package stackport

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

func TestLANGivesTagsNotAddressesAndMatchesTheStacksOwnTags(t *testing.T) {
	ctx := context.Background()
	v := testVault()
	inv := models.DeviceInventory{
		Devices:    []models.NetworkDevice{{MAC: "aa:bb:cc:00:00:10", Name: "Kitchen iPad"}, {MAC: "aa:bb:cc:00:00:01", Name: "Router"}, {MAC: "not-a-mac"}},
		GatewayMAC: "aa:bb:cc:00:00:01",
	}
	got, err := LAN{Vault: v, Inventory: func() (models.DeviceInventory, error) { return inv, nil }}.Devices(ctx, "radio_device")
	if err != nil {
		t.Fatal(err)
	}
	ipad, _ := net.ParseMAC("aa:bb:cc:00:00:10")
	want, _ := v.Tag(ctx, "radio_device", ipad)
	found := false
	for _, d := range got {
		if bytes.Contains(d.Tag, ipad) {
			t.Fatal("an address crossed the port")
		}
		if bytes.Equal(d.Tag, want) && d.Name == "Kitchen iPad" && !d.Router {
			found = true
		}
	}
	if !found {
		t.Error("the stack's own tag of the iPad's address should be in the answer with its name")
	}
	// The radio next to the gateway (one up, and the locally administered twin) is marked as the router's.
	for _, s := range []string{"aa:bb:cc:00:00:02", "a8:bb:cc:00:00:01"} {
		m, _ := net.ParseMAC(s)
		tag, _ := v.Tag(ctx, "radio_device", m)
		ok := false
		for _, d := range got {
			ok = ok || (bytes.Equal(d.Tag, tag) && d.Router && d.Name == "Router")
		}
		if !ok {
			t.Errorf("%s should be marked as the router's", s)
		}
	}
}

func TestLANWithoutAnInventoryIsUnavailable(t *testing.T) {
	_, err := LAN{Vault: testVault(), Inventory: func() (models.DeviceInventory, error) { return models.DeviceInventory{}, errors.New("x") }}.Devices(context.Background(), "radio_device")
	if stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("%v", err)
	}
	got, err := LAN{Vault: testVault(), Inventory: func() (models.DeviceInventory, error) { return models.DeviceInventory{}, nil }}.Devices(context.Background(), "radio_device")
	if err != nil || len(got) != 0 {
		t.Errorf("an empty inventory is an empty answer: %v %v", got, err)
	}
}

func TestOffsetMACCarriesAcrossBytes(t *testing.T) {
	m, _ := net.ParseMAC("aa:bb:cc:00:00:ff")
	if got := offsetMAC(m, 1).String(); got != "aa:bb:cc:00:01:00" {
		t.Errorf("%s", got)
	}
	if got := offsetMAC(m, -0x100).String(); got != "aa:bb:cc:ff:ff:ff" {
		t.Errorf("%s", got)
	}
}
