package persistence

import (
	"os"
	"testing"
	"time"
)

func TestIPOwnersStoreKeepsTheBytesAndTheTimeAndReadsAMissingFileAsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := NewIPOwnersStore(dir)
	if data, at, err := s.Read(); err != nil || data != nil || !at.IsZero() {
		t.Fatalf("a missing file read as %d bytes, %v, %v", len(data), at, err)
	}
	when := time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC)
	if err := s.Write([]byte("table"), when); err != nil {
		t.Fatal(err)
	}
	data, at, err := s.Read()
	if err != nil || string(data) != "table" || !at.Equal(when) {
		t.Fatalf("read %q at %v, %v; want the bytes and the time written", data, at, err)
	}
	if err := s.Write([]byte("newer"), when.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("%d files in the data folder, want one", len(entries))
	}
}

func TestIPOwnersStoreDeleteRemovesTheFileAndNoFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	s := NewIPOwnersStore(dir)
	if err := s.Delete(); err != nil {
		t.Fatalf("deleting nothing: %v", err)
	}
	_ = s.Write([]byte("x"), time.Now())
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("%d files left", len(entries))
	}
}
