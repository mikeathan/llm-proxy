package persistence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func newJournalManager(t *testing.T) *WorkspaceManager {
	t.Helper()
	tmp := t.TempDir()
	return NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
}

func TestWorkspaceManager_Journal(t *testing.T) {
	mgr := newJournalManager(t)

	t.Run("a missing journal reads as empty", func(t *testing.T) {
		if got, err := mgr.ReadJournal("ws", "nightly"); err != nil || got != "" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("write sanitises and round-trips", func(t *testing.T) {
		if err := mgr.WriteJournal("ws", "nightly", "## Worked\x07\n- query A\r\n"); err != nil {
			t.Fatal(err)
		}
		got, err := mgr.ReadJournal("ws", "nightly")
		if err != nil || got != "## Worked\n- query A" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("names that sanitise alike do not share a journal", func(t *testing.T) {
		if got, _ := mgr.ReadJournal("ws", "nightly!"); got != "" {
			t.Errorf("distinct automation names shared a journal: %q", got)
		}
	})

	t.Run("a traversing name stays inside the journal directory", func(t *testing.T) {
		want := filepath.Join(mgr.resolver.InternalDir("ws"), journalDirName) + string(filepath.Separator)
		if p := mgr.journalPath("ws", "../../evil"); !strings.HasPrefix(p, want) {
			t.Errorf("journal path escaped its directory: %s", p)
		}
	})

	t.Run("an oversized journal is capped", func(t *testing.T) {
		if err := mgr.WriteJournal("ws", "big", strings.Repeat("x", models.MaxJournalChars*2)); err != nil {
			t.Fatal(err)
		}
		if got, _ := mgr.ReadJournal("ws", "big"); len([]rune(got)) != models.MaxJournalChars {
			t.Errorf("stored %d characters, want %d", len([]rune(got)), models.MaxJournalChars)
		}
	})

	t.Run("writing empty text clears the journal", func(t *testing.T) {
		if err := mgr.WriteJournal("ws", "nightly", "  \n "); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(mgr.journalPath("ws", "nightly")); !os.IsNotExist(err) {
			t.Errorf("empty write must remove the file, stat err = %v", err)
		}
	})

	t.Run("deleting an automation removes only its journal", func(t *testing.T) {
		_ = mgr.WriteJournal("ws", "gone", "notes")
		_ = mgr.WriteJournal("ws", "kept", "notes")
		if err := mgr.DeleteAutomationRuns("ws", "gone"); err != nil {
			t.Fatal(err)
		}
		if got, _ := mgr.ReadJournal("ws", "gone"); got != "" {
			t.Errorf("deleted automation kept its journal: %q", got)
		}
		if got, _ := mgr.ReadJournal("ws", "kept"); got != "notes" {
			t.Errorf("another automation's journal must survive, got %q", got)
		}
	})
}
