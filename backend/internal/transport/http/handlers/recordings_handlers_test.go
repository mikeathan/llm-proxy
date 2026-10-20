package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/recordings"
)

func recordingsMux(store *recordings.RecordingStore) *http.ServeMux {
	h := NewRecordingHandlers(store, "")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/recordings/content/{id...}", h.Content)
	return mux
}

func writeRun(t *testing.T, root, name string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, "ws", "model-a", "conv_1", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recording.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(filepath.Join("ws", "model-a", "conv_1", name, "recording.jsonl"))
}

func getContent(mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestRecordingContent_ReturnsTheLastRequestOfARun(t *testing.T) {
	root := t.TempDir()
	id := writeRun(t, root, "20261020T190000Z_abc", `{"type":"request","n":1}`, `{"type":"chunk"}`, `{"type":"request","n":2}`, `{"type":"done"}`)
	store, err := recordings.NewRecordingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	mux := recordingsMux(store)

	rec := getContent(mux, "/admin/api/recordings/content/"+id+"?type=request&last=1")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"type":"request","n":2}`+"\n" {
		t.Errorf("last request: %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("content type %q", ct)
	}
	if rec.Header().Get("X-Recording-Truncated") != "" {
		t.Error("nothing was cut")
	}
	whole := getContent(mux, "/admin/api/recordings/content/"+id)
	if strings.Count(whole.Body.String(), "\n") != 4 {
		t.Errorf("without a filter every line comes back: %q", whole.Body.String())
	}
}

func TestRecordingContent_FindsARecordingWrittenAfterTheStoreWasOpened(t *testing.T) {
	root := t.TempDir()
	store, _ := recordings.NewRecordingStore(root)
	mux := recordingsMux(store)
	id := writeRun(t, root, "20261020T200000Z_new", `{"type":"request","n":9}`)
	if rec := getContent(mux, "/admin/api/recordings/content/"+id+"?type=request"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"n":9`) {
		t.Errorf("a new recording must be found: %d %q", rec.Code, rec.Body.String())
	}
}

func TestRecordingContent_Refusals(t *testing.T) {
	root := t.TempDir()
	id := writeRun(t, root, "20261020T190000Z_abc", `{"type":"request"}`)
	store, _ := recordings.NewRecordingStore(root)
	mux := recordingsMux(store)

	cases := []struct {
		name, target string
		want         int
	}{
		{"unknown id", "/admin/api/recordings/content/ws/none/recording.jsonl", http.StatusNotFound},
		{"path traversal is just an unknown id", "/admin/api/recordings/content/..%2F..%2Fetc%2Fpasswd", http.StatusNotFound},
		{"bad type", "/admin/api/recordings/content/" + id + "?type=request%22%7D", http.StatusBadRequest},
		{"bad last", "/admin/api/recordings/content/" + id + "?last=-1", http.StatusBadRequest},
		{"bad max_bytes", "/admin/api/recordings/content/" + id + "?max_bytes=abc", http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := getContent(mux, c.target); rec.Code != c.want {
			t.Errorf("%s: %d, want %d (%q)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}

	nilMux := recordingsMux(nil)
	if rec := getContent(nilMux, "/admin/api/recordings/content/"+id); rec.Code != http.StatusNotFound {
		t.Errorf("no store: %d", rec.Code)
	}
}

func TestRecordingContent_TheCapBoundsTheResponseAndIsFlagged(t *testing.T) {
	root := t.TempDir()
	id := writeRun(t, root, "20261020T190000Z_abc", `{"type":"request","text":"`+strings.Repeat("x", 5000)+`"}`)
	store, _ := recordings.NewRecordingStore(root)
	rec := getContent(recordingsMux(store), "/admin/api/recordings/content/"+id+"?type=request&max_bytes=1000")
	if rec.Body.Len() != 1000 || rec.Header().Get("X-Recording-Truncated") != "true" {
		t.Errorf("len=%d truncated=%q", rec.Body.Len(), rec.Header().Get("X-Recording-Truncated"))
	}
}

// "enabled" is what the Recordings page in the UI keys on (replay, per-automation lists), so it keeps meaning the --record
// flag. Run recordings, which exist whenever run logging is on, are reported separately.
func TestRecordingStatus_EnabledIsTheFlagAndRunRecordingsAreSeparate(t *testing.T) {
	store, _ := recordings.NewRecordingStore(t.TempDir())
	status := func(h *RecordingHandlers) map[string]any {
		rec := httptest.NewRecorder()
		h.Status(rec, httptest.NewRequest(http.MethodGet, "/admin/api/recordings/status", nil))
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	off := status(NewRecordingHandlers(store, ""))
	if off["enabled"] != false || off["dir"] != "" || off["run_recordings"] != true {
		t.Errorf("flag off, store on: %v", off)
	}
	on := status(NewRecordingHandlers(store, "/data/runs"))
	if on["enabled"] != true || on["dir"] != "/data/runs" || on["run_recordings"] != true {
		t.Errorf("flag on: %v", on)
	}
	none := status(NewRecordingHandlers(nil, ""))
	if none["enabled"] != false || none["run_recordings"] != false {
		t.Errorf("no store: %v", none)
	}
}
