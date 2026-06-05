package api

import (
	"fmt"
	"net/http"
	"strconv"

	"llm-proxy/internal/platform/memory"
)

type MemoryHandlers struct {
	store *memory.Store
}

func NewMemoryHandlers(store *memory.Store) *MemoryHandlers {
	return &MemoryHandlers{store: store}
}

func (h *MemoryHandlers) ListMemories(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("workspace")
	if wsID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}
	memType := memory.MemoryType(r.URL.Query().Get("type"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	entries, err := h.store.List(r.Context(), wsID, memType, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("list failed: %v", err))
		return
	}
	if entries == nil {
		entries = []memory.MemoryEntry{}
	}
	respondJSON(w, entries)
}

func (h *MemoryHandlers) SearchMemories(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("workspace")
	if wsID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}

	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Query == "" {
		writeJSONError(w, http.StatusBadRequest, "query is required")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	entries, err := h.store.Search(r.Context(), wsID, req.Query, req.Limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("search failed: %v", err))
		return
	}
	if entries == nil {
		entries = []memory.MemoryEntry{}
	}
	respondJSON(w, entries)
}

func (h *MemoryHandlers) GetMemory(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("workspace")
	idStr := r.PathValue("id")
	if wsID == "" || idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace and id are required")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	entry, err := h.store.Get(r.Context(), wsID, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get failed: %v", err))
		return
	}
	if entry == nil {
		writeJSONError(w, http.StatusNotFound, "memory not found")
		return
	}
	respondJSON(w, entry)
}

func (h *MemoryHandlers) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("workspace")
	idStr := r.PathValue("id")
	if wsID == "" || idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace and id are required")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.store.Update(r.Context(), wsID, id, req.Title, req.Content); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
		return
	}
	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *MemoryHandlers) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("workspace")
	idStr := r.PathValue("id")
	if wsID == "" || idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace and id are required")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Delete(r.Context(), wsID, id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("delete failed: %v", err))
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *MemoryHandlers) ClearWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("workspace")
	if wsID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}
	memType := memory.MemoryType(r.URL.Query().Get("type"))

	n, err := h.store.DeleteAllByWorkspace(r.Context(), wsID, memType)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("clear failed: %v", err))
		return
	}
	respondJSON(w, map[string]any{"status": "cleared", "deleted": n})
}
