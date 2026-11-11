package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestRouterHistoryStore_RoundTripAndMissingFileIsEmpty(t *testing.T) {
	s := NewRouterHistoryStore(t.TempDir())
	got, err := s.Read()
	if err != nil || len(got.Devices) != 0 {
		t.Fatalf("missing file = %+v, %v; want an empty record", got, err)
	}
	at := time.Unix(1, 0).UTC()
	in := models.RouterHistory{Devices: map[string]models.RouterHistoryDevice{
		"aa:bb:cc:00:00:01": {IP: "192.168.50.9", First: at, Last: at, Companies: map[string]models.RouterHistorySeen{"AS13335": {Name: "Cloudflare", First: at, Last: at}}},
	}}
	if err := s.Write(in); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Read(); err != nil || got.Devices["aa:bb:cc:00:00:01"].Companies["AS13335"].Name != "Cloudflare" {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Read(); err != nil || len(got.Devices) != 0 {
		t.Fatalf("after delete = %+v, %v; want empty", got, err)
	}
}
