package persistence

import (
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
