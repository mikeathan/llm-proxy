package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestHostTrafficStoreKeepsOnlyTheLastSummaryAndReadsAMissingOneAsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := NewHostTrafficStore(dir)
	if got, err := s.Read(); err != nil || !got.At.IsZero() {
		t.Fatalf("a missing file read as %+v, %v", got, err)
	}
	first := models.HostTraffic{At: time.Unix(1000, 0).UTC(), Interface: "wlp2s0", BytesOut: 10, Flows: []models.TrafficFlow{{Address: "93.184.216.34", Name: "example.com", Port: 443}}}
	if err := s.Write(first); err != nil {
		t.Fatal(err)
	}
	second := models.HostTraffic{At: time.Unix(2000, 0).UTC(), Interface: "eth0", BytesIn: 20}
	if err := s.Write(second); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil || got.Interface != "eth0" || got.BytesIn != 20 || len(got.Flows) != 0 {
		t.Fatalf("read %+v, %v; the older summary must be replaced, not appended to", got, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("%d files in the data folder, want one", len(entries))
	}
}

func TestHostTrafficStoreSetsACorruptFileAsideOnceAndDeleteRemovesEverything(t *testing.T) {
	dir := t.TempDir()
	s := NewHostTrafficStore(dir)
	if err := os.WriteFile(filepath.Join(dir, hostTrafficFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(); err == nil {
		t.Fatal("a corrupt file read without an error")
	}
	if _, err := os.Stat(filepath.Join(dir, hostTrafficFileName+corruptSuffix)); err != nil {
		t.Fatalf("the corrupt file was not set aside: %v", err)
	}
	if got, err := s.Read(); err != nil || !got.At.IsZero() {
		t.Fatalf("after being set aside the store reads as empty, got %+v, %v", got, err)
	}
	if err := s.Write(models.HostTraffic{At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("%d files left after Delete", len(entries))
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("deleting nothing failed: %v", err)
	}
}
