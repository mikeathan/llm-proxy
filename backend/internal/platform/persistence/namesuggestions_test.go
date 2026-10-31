package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestNameSuggestionStore_RoundTripMissingFileAndDelete(t *testing.T) {
	s := NewNameSuggestionStore(t.TempDir())
	if got, err := s.Read(); err != nil || len(got.Entries) != 0 {
		t.Fatalf("a missing file is an empty list: %+v, %v", got, err)
	}
	want := models.NameSuggestions{Entries: []models.NameSuggestion{{MAC: "aa:bb:cc:00:00:01", Name: "Samsung", TriedAt: time.Unix(5, 0).UTC(), AnsweredAt: time.Unix(5, 0).UTC()}}}
	if err := s.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil || len(got.Entries) != 1 || got.Entries[0] != want.Entries[0] {
		t.Errorf("round trip = %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(); len(got.Entries) != 0 {
		t.Error("a factory reset clears the suggestions")
	}
}
