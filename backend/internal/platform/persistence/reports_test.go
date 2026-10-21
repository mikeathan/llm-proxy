package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
)

func newReportsManager(t *testing.T) (*WorkspaceManager, string) {
	t.Helper()
	tmp := t.TempDir()
	return NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp)), tmp
}

var reportTime = time.Date(2026, 10, 21, 14, 5, 9, 0, time.FixedZone("BST", 3600))

// A final report is saved under reports/<automation>/ in the workspace, named by the run's start time (UTC) and id,
// and the returned path is relative to the workspace so the Files page and read_file can open it.
func TestWriteReport_SavesUnderTheAutomationFolder(t *testing.T) {
	mgr, _ := newReportsManager(t)

	rel, err := mgr.WriteReport("ws", "daily-news", "run_42", reportTime, "# Report\nbody")
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	want := "reports/daily-news/20261021T130509Z-run_42.md"
	if rel != want {
		t.Fatalf("path %q, want %q", rel, want)
	}
	got, err := mgr.ReadTaskFile("ws", rel)
	if err != nil || got != "# Report\nbody" {
		t.Fatalf("read back %q, %v", got, err)
	}
}

// Writing the same run twice replaces the file rather than adding a second one.
func TestWriteReport_SameRunReplacesTheFile(t *testing.T) {
	mgr, _ := newReportsManager(t)
	first, _ := mgr.WriteReport("ws", "daily-news", "run_1", reportTime, "one")
	second, err := mgr.WriteReport("ws", "daily-news", "run_1", reportTime, "two")
	if err != nil || first != second {
		t.Fatalf("paths %q and %q, %v", first, second, err)
	}
	if got, _ := mgr.ReadTaskFile("ws", second); got != "two" {
		t.Fatalf("got %q, want the second write", got)
	}
}

// An automation or run name is data, not a path: separators and dot segments cannot lead out of reports/.
func TestWriteReport_NamesCannotEscapeTheReportsFolder(t *testing.T) {
	mgr, tmp := newReportsManager(t)
	for _, name := range []string{"../../etc", "..", ".", "a/b", `a\b`, "", "nul\x00byte", "héllo wörld"} {
		rel, err := mgr.WriteReport("ws", name, "../run", reportTime, "x")
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		parts := strings.Split(rel, "/")
		if len(parts) != 3 || parts[0] != "reports" {
			t.Errorf("%q gave %q: not one folder under reports/", name, rel)
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				t.Errorf("%q gave %q: segment %q leads out of the folder", name, rel, part)
			}
		}
	}
	// nothing was written next to the workspace or above it
	entries, _ := os.ReadDir(filepath.Join(tmp))
	for _, e := range entries {
		if e.Name() == "etc" || e.Name() == "x.md" {
			t.Errorf("stray %s outside the workspace", e.Name())
		}
	}
}

func writeReportAt(t *testing.T, mgr *WorkspaceManager, automation, runID string, at time.Time, body string) string {
	t.Helper()
	rel, err := mgr.WriteReport("ws", automation, runID, at, body)
	if err != nil {
		t.Fatalf("WriteReport %s/%s: %v", automation, runID, err)
	}
	return rel
}

// The list holds one automation's saved reports, newest first, each with its run id, start time and size; another
// automation's reports are not in it.
func TestListReports_NewestFirstForOneAutomation(t *testing.T) {
	mgr, _ := newReportsManager(t)
	day := time.Date(2026, 10, 19, 7, 0, 0, 0, time.UTC)
	oldest := writeReportAt(t, mgr, "daily news", "run_1", day, "one")
	newest := writeReportAt(t, mgr, "daily news", "run_3", day.Add(48*time.Hour), "three three")
	middle := writeReportAt(t, mgr, "daily news", "run_2", day.Add(24*time.Hour), "two")
	writeReportAt(t, mgr, "other", "run_9", day, "not ours")

	list, err := mgr.ListReports("ws", "daily news")
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if list.Truncated || len(list.Reports) != 3 {
		t.Fatalf("got %+v", list)
	}
	for i, want := range []struct {
		path, runID string
		size        int64
		at          time.Time
	}{
		{newest, "run_3", 11, day.Add(48 * time.Hour)},
		{middle, "run_2", 3, day.Add(24 * time.Hour)},
		{oldest, "run_1", 3, day},
	} {
		got := list.Reports[i]
		if got.Path != want.path || got.RunID != want.runID || got.SizeBytes != want.size || !got.StartedAt.Equal(want.at) {
			t.Errorf("entry %d = %+v, want %+v", i, got, want)
		}
	}
}

// An automation that never saved a report has an empty list, and the list is never nil (it is sent as []).
func TestListReports_NoneSavedIsEmpty(t *testing.T) {
	mgr, _ := newReportsManager(t)
	for _, ws := range []string{"ws", "no-such-workspace"} {
		list, err := mgr.ListReports(ws, "never-ran")
		if err != nil || list.Reports == nil || len(list.Reports) != 0 || list.Truncated {
			t.Errorf("%s: %+v, %v", ws, list, err)
		}
	}
}

// Only files the system wrote are listed: a note the user dropped in the folder, a sub-folder and a file with a
// malformed name are left out.
func TestListReports_SkipsWhatTheSystemDidNotWrite(t *testing.T) {
	mgr, _ := newReportsManager(t)
	kept := writeReportAt(t, mgr, "nightly", "run_1", reportTime, "report")
	dir := filepath.Dir(filepath.Join(mgr.resolver.WorkspaceDir("ws"), filepath.FromSlash(kept)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"notes.md", "20261021T130509Z-run_1.txt", "bad-stamp-run_2.md", "20261321T130509Z-run_3.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "20261021T130509Z-run_4.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := mgr.ListReports("ws", "nightly")
	if err != nil || len(list.Reports) != 1 || list.Reports[0].Path != kept {
		t.Fatalf("got %+v, %v", list, err)
	}
}

// The list is bounded: past MaxListedReports the newest are kept and the list says it was cut.
func TestListReports_IsBoundedToTheNewest(t *testing.T) {
	mgr, _ := newReportsManager(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	total := MaxListedReports + 5
	for i := 0; i < total; i++ {
		writeReportAt(t, mgr, "busy", fmt.Sprintf("run_%d", i), start.Add(time.Duration(i)*time.Hour), "x")
	}
	list, err := mgr.ListReports("ws", "busy")
	if err != nil || !list.Truncated || len(list.Reports) != MaxListedReports {
		t.Fatalf("len %d truncated %v, %v", len(list.Reports), list.Truncated, err)
	}
	if want := start.Add(time.Duration(total-1) * time.Hour); !list.Reports[0].StartedAt.Equal(want) {
		t.Errorf("first is %v, want the newest %v", list.Reports[0].StartedAt, want)
	}
}
