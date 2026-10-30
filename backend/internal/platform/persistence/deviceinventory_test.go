package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestDeviceInventoryStore_RoundTripAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	s := NewDeviceInventoryStore(dir)
	if inv, err := s.Read(); err != nil || inv.Baselined || len(inv.Devices) != 0 {
		t.Fatalf("a missing file is an empty inventory: %+v, %v", inv, err)
	}
	want := models.DeviceInventory{Baselined: true, GatewayMAC: "aa:bb:cc:00:00:01", Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.1", Name: "Router", Approved: true, FirstSeen: time.Unix(1, 0).UTC(), LastSeen: time.Unix(2, 0).UTC()},
	}}
	if err := s.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil || len(got.Devices) != 1 || got.Devices[0].Name != "Router" || got.GatewayMAC != want.GatewayMAC || !got.Baselined {
		t.Errorf("round trip = %+v, %v", got, err)
	}
}

func TestDeviceInventoryStore_CorruptFileIsSetAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, deviceInventoryFileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDeviceInventoryStore(dir).Read(); err == nil {
		t.Fatal("a corrupt file is an error")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the corrupt file is kept as evidence: %v", err)
	}
}

func TestDeviceInventoryStore_DeleteRemovesTheFileAndToleratesNone(t *testing.T) {
	dir := t.TempDir()
	s := NewDeviceInventoryStore(dir)
	if err := s.Delete(); err != nil {
		t.Fatalf("deleting nothing is not an error: %v", err)
	}
	if err := s.Write(models.DeviceInventory{Baselined: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if inv, _ := s.Read(); inv.Baselined {
		t.Error("the inventory is gone")
	}
}
