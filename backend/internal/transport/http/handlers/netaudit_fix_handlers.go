package handlers

import (
	"errors"
	"net/http"

	"llm-proxy/internal/core/netaudit"
)

// fixRequest records that the operator applied the fix for one finding of an automation's latest scan.
type fixRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Automation  string `json:"automation"`
	Key         string `json:"key"`
	Note        string `json:"note"`
}

// MarkFixed records "I've done this" for a finding. It changes nothing about the finding: the next scan decides whether the
// fix worked.
func (h *NetauditHandlers) MarkFixed(w http.ResponseWriter, r *http.Request) {
	var req fixRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.WorkspaceID == "" || req.Automation == "" || req.Key == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id, automation and key are required")
		return
	}
	rec, err := h.fixes.Mark(req.WorkspaceID, req.Automation, req.Key, req.Note)
	switch {
	case errors.Is(err, netaudit.ErrBadFix):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, netaudit.ErrNoSuchFinding):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	default:
		respondJSON(w, rec)
	}
}

// UnmarkFixed takes a fix record back.
func (h *NetauditHandlers) UnmarkFixed(w http.ResponseWriter, r *http.Request) {
	workspaceID, key := r.URL.Query().Get("workspace_id"), r.URL.Query().Get("key")
	if workspaceID == "" || key == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id and key are required")
		return
	}
	ok, err := h.fixes.Unmark(workspaceID, key)
	switch {
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	case !ok:
		writeJSONError(w, http.StatusNotFound, "no fix record with that key")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
