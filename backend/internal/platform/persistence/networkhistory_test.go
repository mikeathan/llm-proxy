package persistence

import (
	"testing"

	"llm-proxy/models"
)

func TestNetworkHistoryStore_RoundTripMissingFileAndDelete(t *testing.T) {
	s := NewNetworkHistoryStore(t.TempDir())
	if h, err := s.Read(); err != nil || len(h.Days) != 0 {
		t.Fatalf("a missing file is an empty history: %+v, %v", h, err)
	}
	want := models.NetworkHistory{Days: []models.NetworkDay{{Date: "2026-10-31", DevicesUp: 5, Devices: 9, Unnamed: 2, Findings: 3, Serious: 1, Audited: true}}}
	if err := s.Write(want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Read(); err != nil || len(got.Days) != 1 || got.Days[0] != want.Days[0] {
		t.Errorf("round trip = %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if h, _ := s.Read(); len(h.Days) != 0 {
		t.Error("a factory reset clears the history")
	}
}
