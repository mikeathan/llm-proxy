package handlers

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"

	"llm-proxy/internal/recordings"
)

type RecordingHandlers struct {
	store *recordings.RecordingStore
	// recordDir is the --record directory, "" when the flag is off. The store lists run recordings either way.
	recordDir string
}

func NewRecordingHandlers(store *recordings.RecordingStore, recordDir string) *RecordingHandlers {
	return &RecordingHandlers{store: store, recordDir: recordDir}
}

func (h *RecordingHandlers) List(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondJSON(w, []recordings.RecordingMeta{})
		return
	}
	if err := h.store.Refresh(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	automation := r.URL.Query().Get("automation")
	var list []recordings.RecordingMeta
	if automation != "" {
		list = h.store.ListByAutomation(automation)
	} else {
		list = h.store.List()
	}
	respondJSON(w, list)
}

// Status reports whether --record is on ("enabled" and its "dir", which the Recordings page in the UI keys on) and,
// separately, whether run recordings can be listed and read ("run_recordings"): they exist whenever run logging is on.
func (h *RecordingHandlers) Status(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]interface{}{
		"enabled":        h.recordDir != "",
		"dir":            h.recordDir,
		"run_recordings": h.store != nil,
	})
}

func (h *RecordingHandlers) Get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondError(w, http.StatusNotFound, "recording store not available")
		return
	}
	vals, ok := requirePathParams(w, r, "id")
	if !ok {
		return
	}
	id := vals[0]
	meta, ok := h.store.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "recording not found")
		return
	}
	respondJSON(w, meta)
}

func (h *RecordingHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondError(w, http.StatusNotFound, "recording store not available")
		return
	}
	vals, ok := requirePathParams(w, r, "id")
	if !ok {
		return
	}
	id := vals[0]
	if err := h.store.Delete(id); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

const (
	defaultContentBytes = 1 << 20
	maxContentBytes     = 16 << 20
)

// recordingLineType is what "type" may be: the recorder's line types ("request", "chunk", "response", "done", "error").
var recordingLineType = regexp.MustCompile(`^[a-z_]{1,32}$`)

// Content returns lines of one recording as JSON lines, so the exact request a model received can be read without access
// to the machine: ?type=request&last=1 is the last request, ?max_bytes bounds the response (default 1 MiB, at most
// 16 MiB) and X-Recording-Truncated says when it cut anything. The recording is found by its listed id, never by joining
// the id to a path.
func (h *RecordingHandlers) Content(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondError(w, http.StatusNotFound, "recording store not available")
		return
	}
	lineType, last, maxBytes, ok := contentParams(w, r)
	if !ok {
		return
	}
	id := filepath.FromSlash(r.PathValue("id"))
	meta, found := h.store.Get(id)
	if !found {
		if err := h.store.Refresh(); err == nil { // a run may have written its recording after the store last looked
			meta, found = h.store.Get(id)
		}
	}
	if !found {
		respondError(w, http.StatusNotFound, "recording not found")
		return
	}
	data, truncated, err := recordings.ReadLines(meta.FilePath, lineType, last, maxBytes)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not read the recording")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	if truncated {
		w.Header().Set("X-Recording-Truncated", "true")
	}
	_, _ = w.Write(data)
}

// contentParams reads and validates the query of Content; false means a 400 has been written.
func contentParams(w http.ResponseWriter, r *http.Request) (lineType string, last, maxBytes int, ok bool) {
	q := r.URL.Query()
	lineType = q.Get("type")
	if lineType != "" && !recordingLineType.MatchString(lineType) {
		respondError(w, http.StatusBadRequest, "type must be a line type such as request")
		return "", 0, 0, false
	}
	last, maxBytes = 0, defaultContentBytes
	if v := q.Get("last"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			respondError(w, http.StatusBadRequest, "last must be a whole number, 0 or more")
			return "", 0, 0, false
		}
		last = n
	}
	if v := q.Get("max_bytes"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			respondError(w, http.StatusBadRequest, "max_bytes must be a whole number, 1 or more")
			return "", 0, 0, false
		}
		maxBytes = min(n, maxContentBytes)
	}
	return lineType, last, maxBytes, true
}
