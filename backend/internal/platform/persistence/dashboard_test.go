package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/models"
)

func TestDashboardStore_RoundTripsAndForgets(t *testing.T) {
	dir := t.TempDir()
	s := NewDashboardStore(dir)
	if got, err := s.Read(); err != nil || len(got.Boards) != 0 {
		t.Fatalf("a missing file is an empty layout, got %+v, %v", got, err)
	}
	in := models.Dashboard{Boards: []models.DashboardBoard{{ID: "home", Name: "Home", Cards: []models.DashboardCard{{ID: "radar", Cols: 8, Rows: 2}}}}}
	if err := s.Write(in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil || len(got.Boards) != 1 || got.Boards[0].Cards[0].Cols != 8 {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, dashboardFileName)); !os.IsNotExist(err) {
		t.Fatalf("the file must be gone after Delete, stat error = %v", err)
	}
}

func TestDashboardStore_AnUndecodableFileIsSetAsideAndReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dashboardFileName), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDashboardStore(dir).Read(); err == nil {
		t.Fatal("an undecodable layout must be reported so the next save does not silently replace it")
	}
	if _, err := os.Stat(filepath.Join(dir, dashboardFileName+corruptSuffix)); err != nil {
		t.Fatalf("the damaged file must be kept as *.corrupt: %v", err)
	}
}
