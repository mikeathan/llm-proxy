package handlers

import (
	"context"
	"errors"
	"net/http"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/scanscope"
)

// NetauditRechecker re-runs what produced one finding and says whether it is gone. *netaudit.Recheck satisfies it.
type NetauditRechecker interface {
	Check(ctx context.Context, workspaceID, automation, key string) (netaudit.RecheckResult, error)
}

// WithRecheck lets the operator ask "is it fixed now?" for one finding.
func (h *NetauditHandlers) WithRecheck(r NetauditRechecker) *NetauditHandlers {
	h.recheck = r
	return h
}

// RecheckFinding re-checks one finding of an automation's latest scan on its own device and answers fixed, still there or could
// not check. A fixed answer goes to the finding's history; nothing else is changed.
func (h *NetauditHandlers) RecheckFinding(w http.ResponseWriter, r *http.Request) {
	var req fixRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.WorkspaceID == "" || req.Automation == "" || req.Key == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id, automation and key are required")
		return
	}
	if h.recheck == nil {
		writeJSONError(w, http.StatusNotFound, "checking again is not available")
		return
	}
	res, err := h.recheck.Check(r.Context(), req.WorkspaceID, req.Automation, req.Key)
	switch {
	case errors.Is(err, netaudit.ErrNoSuchFinding):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, netaudit.ErrScanBusy):
		writeJSONError(w, http.StatusConflict, "A scan is running. Try again when it has finished.")
	case errors.Is(err, netaudit.ErrUseDeepCheck), errors.Is(err, netaudit.ErrNotRecheckable), errors.Is(err, scanscope.ErrOutOfScope):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	default:
		respondJSON(w, res)
	}
}
