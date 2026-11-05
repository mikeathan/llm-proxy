package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestPhoneHomeStore_RoundTripAndMissingFileIsEmpty(t *testing.T) {
	s := NewPhoneHomeStore(t.TempDir())
	got, err := s.Read()
	if err != nil || len(got.Devices) != 0 {
		t.Fatalf("missing file = %+v, %v; want an empty record", got, err)
	}
	in := models.PhoneHome{Since: time.Unix(1, 0).UTC(), Devices: []models.PhoneHomeDevice{{MAC: "aa:bb:cc:00:00:01", Names: []models.PhoneHomeName{{Name: "example.com", Count: 2}}}}}
	if err := s.Write(in); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Read(); err != nil || len(got.Devices) != 1 || got.Devices[0].Names[0].Name != "example.com" {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
}
