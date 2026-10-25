package handlers

import (
	"context"
	"errors"
	"net/http"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/scanscope"
)

// NetauditScope is the part of the scan-scope service the handlers use.
type NetauditScope interface {
	View() netaudit.ScopeView
	SetRanges(ctx context.Context, entries []string) (netaudit.ScopeView, error)
	SetAllowUnconfined(allow bool) error
	Suggestions() ([]netaudit.Suggestion, error)
}

// NetauditHandlers serves the operator's network-audit settings.
type NetauditHandlers struct {
	scope NetauditScope
}

func NewNetauditHandlers(scope NetauditScope) *NetauditHandlers {
	return &NetauditHandlers{scope: scope}
}

// scopeResponse is the scan scope with the networks the machine is attached to, so the settings page can offer them.
type scopeResponse struct {
	netaudit.ScopeView
	Suggestions []netaudit.Suggestion `json:"suggestions"`
}

// scopeRequest changes the scan scope. A nil field is left as it is.
type scopeRequest struct {
	Entries         *[]string `json:"entries"`
	AllowUnconfined *bool     `json:"allow_unconfined"`
}

// GetScope returns the declared scan ranges and the networks this machine could scan.
func (h *NetauditHandlers) GetScope(w http.ResponseWriter, _ *http.Request) {
	h.respondScope(w, h.scope.View())
}

// PutScope replaces the declared ranges and/or the unconfined switch. An entry the policy refuses (a public
// address, a name that does not resolve) is a 400 that names it, and nothing is saved.
func (h *NetauditHandlers) PutScope(w http.ResponseWriter, r *http.Request) {
	var req scopeRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	view := h.scope.View()
	if req.Entries != nil {
		updated, err := h.scope.SetRanges(r.Context(), *req.Entries)
		if err != nil {
			status := http.StatusInternalServerError
			if isScopeRefusal(err) {
				status = http.StatusBadRequest
			}
			writeJSONError(w, status, err.Error())
			return
		}
		view = updated
	}
	if req.AllowUnconfined != nil {
		if err := h.scope.SetAllowUnconfined(*req.AllowUnconfined); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view = h.scope.View()
	}
	h.respondScope(w, view)
}

func (h *NetauditHandlers) respondScope(w http.ResponseWriter, view netaudit.ScopeView) {
	suggestions, err := h.scope.Suggestions()
	if err != nil {
		suggestions = nil // a failure to list interfaces must not hide the saved scope
	}
	if suggestions == nil {
		suggestions = []netaudit.Suggestion{}
	}
	respondJSON(w, scopeResponse{ScopeView: view, Suggestions: suggestions})
}

// isScopeRefusal reports whether err is the operator's input being refused rather than a failure to save.
func isScopeRefusal(err error) bool {
	return errors.Is(err, scanscope.ErrBadEntry) || errors.Is(err, scanscope.ErrNotScannable)
}
