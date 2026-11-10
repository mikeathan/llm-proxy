package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestFootprintStore_RoundTripAndDelete(t *testing.T) {
	s := NewFootprintStore(t.TempDir())
	if got, err := s.Read(); err != nil || len(got) != 0 {
		t.Fatalf("none yet: %v %v", got, err)
	}
	in := []models.FootprintResult{{Watch: "domain:example.com", Check: models.FootprintCheckCerts, Status: models.FootprintLooked, CertIDs: []int64{1, 2}, At: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)}}
	if err := s.Write(in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil || len(got) != 1 || got[0].CertIDs[1] != 2 {
		t.Errorf("%v %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(); len(got) != 0 {
		t.Errorf("a factory reset must remove them: %v", got)
	}
}
