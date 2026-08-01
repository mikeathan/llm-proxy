package handlers

import (
	"net/http"

	"llm-proxy/internal/recordings"
)

type RecordingHandlers struct {
	store *recordings.RecordingStore
}

func NewRecordingHandlers(store *recordings.RecordingStore) *RecordingHandlers {
	return &RecordingHandlers{store: store}
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

func (h *RecordingHandlers) Status(w http.ResponseWriter, r *http.Request) {
	enabled := h.store != nil
	dir := ""
	if enabled {
		dir = h.store.RecordDir()
	}
	respondJSON(w, map[string]interface{}{
		"enabled": enabled,
		"dir":     dir,
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


