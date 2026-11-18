package stackport

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

func TestMeshGivesTagsNotAddressesNamesTheBoxesAndMarksTheMainOne(t *testing.T) {
	ctx := context.Background()
	v := testVault()
	at := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	list := &models.MeshClients{At: at, Clients: []models.MeshClient{
		{MAC: "0a:00:00:00:00:90", Box: "02:00:00:00:00:f0", Band: "5G", RSSI: -57},
		{MAC: "0a:00:00:00:00:23", Box: "24:00:00:00:00:60", Band: "2G", Guest: true, RSSI: -61},
	}}
	inv := models.DeviceInventory{
		Devices:    []models.NetworkDevice{{MAC: "02:00:00:00:00:F0", Name: "Mesh, first floor"}, {MAC: "24:00:00:00:00:60", Name: "Asus router"}},
		GatewayMAC: "24-00-00-00-00-60",
	}
	got, err := Mesh{Vault: v, Read: func(context.Context) (*models.MeshClients, error) { return list, nil },
		Inventory: func() (models.DeviceInventory, error) { return inv, nil }}.Clients(ctx, "radio_device")
	if err != nil || len(got.Devices) != 2 || !got.At.Equal(at) {
		t.Fatalf("%+v %v", got, err)
	}
	phone, _ := net.ParseMAC("0a:00:00:00:00:90")
	box, _ := net.ParseMAC("02:00:00:00:00:f0")
	wantPhone, _ := v.Tag(ctx, "radio_device", phone)
	wantBox, _ := v.Tag(ctx, "radio_device", box)
	d := got.Devices[0]
	if !bytes.Equal(d.Tag, wantPhone) || !bytes.Equal(d.Box, wantBox) || d.BoxName != "Mesh, first floor" || d.Main || d.RSSI != -57 || d.Band != "5G" {
		t.Errorf("the device and its box as the stack's own tags, the box named from the inventory: %+v", d)
	}
	if g := got.Devices[1]; !g.Main || g.BoxName != "Asus router" || !g.Guest {
		t.Errorf("the gateway's box is the main one, whatever the address's spelling: %+v", g)
	}
	for _, d := range got.Devices {
		if bytes.Contains(d.Tag, phone) || bytes.Contains(d.Box, box) {
			t.Fatal("an address crossed the port")
		}
	}
}

func TestMeshThatCannotBeReadIsUnavailable(t *testing.T) {
	_, err := Mesh{Vault: testVault(), Read: func(context.Context) (*models.MeshClients, error) { return nil, errors.New("router gone") }}.Clients(context.Background(), "radio_device")
	if stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("%v", err)
	}
	if _, err := (Mesh{}).Clients(context.Background(), "radio_device"); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("no read wired: %v", err)
	}
}
