package agentwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanSecrets_FindsAKeyByPlaceNeverByValueAndOnlyOnce(t *testing.T) {
	w, store, out, _ := newTestWatch()
	dir := t.TempDir()
	token := "lp_" + strings.Repeat("b", 52)
	writeFile(t, filepath.Join(dir, "reports", "news.md"), "The key is "+token+" for later.")
	writeFile(t, filepath.Join(dir, "notes.txt"), "nothing secret")
	writeFile(t, filepath.Join(dir, ".internal", "state.json"), token) // the program's own files are not agent output
	writeFile(t, filepath.Join(dir, "node_modules", "x.js"), token)    // nor a dependency folder
	w.deps.Workspaces = func() ([]WorkspaceDir, error) { return []WorkspaceDir{{ID: "ws", Dir: dir}}, nil }

	w.scanSecrets(t.Context())
	got := store.rec.Events
	if len(got) != 1 || got[0].Kind != models.AgentSecretFound || got[0].Where != "reports/news.md" || got[0].Detail != "a script token of this program" {
		t.Fatalf("events = %+v", got)
	}
	msgs := out.all()
	if len(msgs) != 1 || strings.Contains(msgs[0], token) || strings.Contains(strings.Join([]string{got[0].Where, got[0].Detail}, " "), token) {
		t.Fatalf("alerts %q; the value must never leave the file", msgs)
	}
	w.scanSecrets(t.Context())
	if len(store.rec.Events) != 1 || len(out.all()) != 1 {
		t.Error("a secret already reported is not reported again")
	}
}

func TestScanSecrets_SkipsLinksAndLargeFiles(t *testing.T) {
	w, store, _, _ := newTestWatch()
	dir, outside := t.TempDir(), t.TempDir()
	token := "lp_" + strings.Repeat("c", 52)
	writeFile(t, filepath.Join(outside, "secret.txt"), token)
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Skip("no symlinks here")
	}
	writeFile(t, filepath.Join(dir, "big.log"), strings.Repeat("x", maxScanFile)+token)
	w.deps.Workspaces = func() ([]WorkspaceDir, error) { return []WorkspaceDir{{ID: "ws", Dir: dir}}, nil }
	w.scanSecrets(t.Context())
	if len(store.rec.Events) != 0 {
		t.Errorf("events = %+v; a link is not followed and a file over the size limit is not read", store.rec.Events)
	}
}

func TestForgetOldest_KeepsTheNewest(t *testing.T) {
	t0 := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	seen := map[string]time.Time{"a": t0, "b": t0.Add(time.Hour), "c": t0.Add(2 * time.Hour)}
	forgetOldest(seen, 2)
	if _, ok := seen["a"]; ok || len(seen) != 2 {
		t.Errorf("seen = %v; want the oldest gone", seen)
	}
}

func TestScanSecrets_DoesNothingWhileThePassIsSwitchedOff(t *testing.T) {
	w, store, out, _ := newTestWatch()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes.md"), "key lp_"+strings.Repeat("d", 52))
	w.deps.Workspaces = func() ([]WorkspaceDir, error) { return []WorkspaceDir{{ID: "ws", Dir: dir}}, nil }
	on := false
	w.deps.SecretsPassOn = func() bool { return on }

	w.scanSecrets(t.Context())
	if len(store.rec.Events) != 0 || len(out.all()) != 0 {
		t.Fatalf("the pass ran while switched off: %+v %q", store.rec.Events, out.all())
	}
	on = true
	w.scanSecrets(t.Context())
	if len(store.rec.Events) != 1 {
		t.Fatalf("events = %+v; the pass must run once it is switched on", store.rec.Events)
	}
}
