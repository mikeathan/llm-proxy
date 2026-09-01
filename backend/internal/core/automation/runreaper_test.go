package automation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRunReaper_PrunesOldRuns verifies the reaper removes run directories
// older than the retention window, keeps fresh ones, and cleans up the
// now-empty workspace/model/task parents.
func TestRunReaper_PrunesOldRuns(t *testing.T) {
	root := t.TempDir()
	runsRoot := filepath.Join(root, "runs")

	old := filepath.Join(runsRoot, "ws-1", "model-a", "task-b", "20200101T000000Z_aaaa")
	fresh := filepath.Join(runsRoot, "ws-1", "model-a", "task-b", time.Now().UTC().Format("20060102T150405Z")+"_bbbb")
	for _, d := range []string{old, fresh} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
		// A completed run dir always contains at least run-meta.json, so the
		// empty-parent sweep must not mistake it for a leftover dir.
		if err := os.WriteFile(filepath.Join(d, "run-meta.json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("write meta: %v", err)
		}
	}

	reaper := NewRunReaper(runsRoot, time.Hour, 24*time.Hour)
	reaper.prune(context.Background())

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old run dir to be pruned, stat err=%v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("expected fresh run dir to be kept: %v", err)
	}
	// Empty parents above the fresh run are kept; the old run's now-empty
	// ancestors may be removed — verify the runs root still exists.
	if _, err := os.Stat(runsRoot); err != nil {
		t.Errorf("expected runs root to remain: %v", err)
	}
}

// TestRunReaper_IgnoresNonRunDirs verifies ordinary directories are never
// treated as run dirs even when they are old.
func TestRunReaper_IgnoresNonRunDirs(t *testing.T) {
	root := t.TempDir()
	runsRoot := filepath.Join(root, "runs")
	plain := filepath.Join(runsRoot, "ws-1", "notes")
	if err := os.MkdirAll(plain, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, "keep.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	reaper := NewRunReaper(runsRoot, time.Hour, 0) // retention 0 → everything old
	reaper.prune(context.Background())

	if _, err := os.Stat(plain); err != nil {
		t.Errorf("expected non-run dir to be untouched: %v", err)
	}
}
