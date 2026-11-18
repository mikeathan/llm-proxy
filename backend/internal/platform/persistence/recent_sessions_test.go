package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
)

// RecentSessionIDs lists the newest conversations first from the files' times, at most max, without decoding any of them.
func TestRecentSessionIDs_NewestFirstAndBounded(t *testing.T) {
	root := t.TempDir()
	m := NewWorkspaceManager(storage.NewPathResolver(root, filepath.Join(root, "ws"), filepath.Join(root, "meta")))
	dir := m.resolver.SessionsDir("w")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"old", "mid", "new"} {
		p := filepath.Join(dir, id+".json")
		if err := os.WriteFile(p, []byte("not json at all"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, base, base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	got := m.RecentSessionIDs("w", 2)
	if len(got) != 2 || got[0] != "new" || got[1] != "mid" {
		t.Errorf("got %v, want [new mid]", got)
	}
}
