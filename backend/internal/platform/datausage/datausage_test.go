package datausage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func touch(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
}

func dirSize(dir string, _ time.Time) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// Every top-level entry is a row: the named ones under their kind, grouped, and anything no kind names under its own name, so nothing hides.
func TestMeasure_NamesWhatItKnowsAndHidesNothing(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "settings.yml"), 10)
	touch(t, filepath.Join(root, "orchestrator.db"), 100)
	touch(t, filepath.Join(root, "orchestrator.db-wal"), 50)
	touch(t, filepath.Join(root, "runs", "ws", "a.jsonl"), 300)
	touch(t, filepath.Join(root, "mystery.bin"), 7)
	kinds := []Kind{
		{ID: "settings", Title: "Settings", Owner: "core", Names: []string{"settings.yml"}},
		{ID: "db", Title: "Database", Owner: "core", Names: []string{"orchestrator.db*"}},
		{ID: "runs", Title: "Runs", Owner: "core", Names: []string{"runs"}, Clean: "Clear runtime data"},
	}
	rows, err := Measure(root, kinds, dirSize, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if r := byID["db"]; r.Bytes != 150 || len(r.Entries) != 2 {
		t.Errorf("the database and its journal are one row: %+v", r)
	}
	if r := byID["runs"]; r.Bytes != 300 || r.Clean != "Clear runtime data" {
		t.Errorf("a folder is measured whole: %+v", r)
	}
	other := byID["other:mystery.bin"]
	if other.Described || other.Title != "mystery.bin" || other.Bytes != 7 {
		t.Errorf("an entry no kind names is a row of its own, not described: %+v", other)
	}
	if rows[len(rows)-1].ID != "other:mystery.bin" {
		t.Errorf("the described rows come first: %+v", rows)
	}
}

// A folder that cannot be read is an error, never an empty report.
func TestMeasure_AnUnreadableRootIsAnError(t *testing.T) {
	if _, err := Measure(filepath.Join(t.TempDir(), "absent"), nil, nil, time.Now()); err == nil {
		t.Error("want an error")
	}
}
