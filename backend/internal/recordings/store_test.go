package recordings

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, dir string, parts ...string) string {
	t.Helper()
	path := filepath.Join(parts...)
	abs := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(`{"type":"request","model":"test"}`+"\n"+`{"type":"response"}`+"\n"+`{"type":"done","total_chunks":1}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRecordingStore_Empty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewRecordingStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 0 {
		t.Fatalf("expected empty store, got %d entries", len(list))
	}
}

func TestRecordingStore_Scan(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "gemma4", "daily-sync", "20260524T120000Z_run_abc.jsonl")
	writeFixture(t, dir, "gemma4", "daily-sync", "20260524T130000Z_run_def.jsonl")
	writeFixture(t, dir, "claude", "report-gen", "20260523T120000Z_run_ghi.jsonl")

	s, err := NewRecordingStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}

	byAuto := s.ListByAutomation("daily-sync")
	if len(byAuto) != 2 {
		t.Fatalf("expected 2 daily-sync entries, got %d", len(byAuto))
	}

	meta, ok := s.Get("gemma4/daily-sync/20260524T120000Z_run_abc.jsonl")
	if !ok {
		t.Fatal("expected to find recording by ID")
	}
	if meta.Model != "gemma4" {
		t.Fatalf("expected model gemma4, got %s", meta.Model)
	}
	if meta.AutomationName != "daily-sync" {
		t.Fatalf("expected automation daily-sync, got %s", meta.AutomationName)
	}
	if meta.SessionID != "run_abc" {
		t.Fatalf("expected session run_abc, got %s", meta.SessionID)
	}
}

func TestRecordingStore_Delete(t *testing.T) {
	dir := t.TempDir()
	id := writeFixture(t, dir, "gemma4", "test", "20260524T120000Z_run_abc.jsonl")

	s, err := NewRecordingStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list := s.List()
	if len(list) != 0 {
		t.Fatalf("expected 0 entries after delete, got %d", len(list))
	}

	if _, err := os.Stat(filepath.Join(dir, id)); !os.IsNotExist(err) {
		t.Fatal("file should be deleted from disk")
	}
}

func TestRecordingStore_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := NewRecordingStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent recording")
	}
}

func TestRecordingStore_Refresh(t *testing.T) {
	dir := t.TempDir()
	s, err := NewRecordingStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	writeFixture(t, dir, "gemma4", "test", "20260524T120000Z_run_abc.jsonl")
	if err := s.Refresh(); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry after refresh, got %d", len(list))
	}
}

// A run folder holds events.jsonl (the event stream, tens of MB for a long run) next to recording.jsonl; only the latter is
// a recording, and listing both doubled every run in the API.
func TestRecordingStore_ListsRecordingsAndNotEventStreams(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "ws", "model-a", "conv_1", "20261020T190000Z_abc")
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"recording.jsonl", "events.jsonl", "run-meta.json"} {
		if err := os.WriteFile(filepath.Join(run, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, root, "model-b", "task", "20261020T180000Z_run_def.jsonl")

	store, err := NewRecordingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range store.List() {
		names = append(names, filepath.Base(m.FilePath))
	}
	if len(names) != 2 {
		t.Fatalf("want the nested recording and the flat one, got %v", names)
	}
	for _, n := range names {
		if n == "events.jsonl" {
			t.Errorf("an event stream is not a recording: %v", names)
		}
	}
}
