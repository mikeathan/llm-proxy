package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestDeviceIdentityStore_RoundTripMissingAndDelete(t *testing.T) {
	s := NewDeviceIdentityStore(t.TempDir())
	if got, err := s.Read(); err != nil || len(got) != 0 {
		t.Fatalf("missing = %+v, %v", got, err)
	}
	want := []models.DeviceIdentity{{Device: "10.0.0.5", Port: 22, Kind: "ssh_hostkey", Prints: []string{"RSA:aa"}, Seen: time.Unix(5, 0).UTC(), Change: &models.IdentityChange{At: time.Unix(6, 0).UTC()}}}
	if err := s.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil || len(got) != 1 || got[0].Change == nil || got[0].Prints[0] != "RSA:aa" {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(); len(got) != 0 {
		t.Error("deleted")
	}
}
