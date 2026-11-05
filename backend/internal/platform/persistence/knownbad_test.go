package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestKnownBadStore_RoundTripAndMissingFileIsEmpty(t *testing.T) {
	s := NewKnownBadStore(t.TempDir())
	got, err := s.Read()
	if err != nil || len(got.Names) != 0 {
		t.Fatalf("missing file = %+v, %v; want an empty list", got, err)
	}
	if err := s.Write(models.KnownBad{LoadedAt: time.Unix(1, 0).UTC(), Names: []string{"a.example"}}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Read(); err != nil || len(got.Names) != 1 || got.Names[0] != "a.example" {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
}
