package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

// NetauditScope is the part of the scan-scope service the handlers use.
type NetauditScope interface {
	View() netaudit.ScopeView
	SetRanges(ctx context.Context, entries []string) (netaudit.ScopeView, error)
	SetAllowUnconfined(allow bool) error
	Suggestions() ([]netaudit.Suggestion, error)
}

// NetauditReadiness answers what is missing before scans can run.
type NetauditReadiness interface {
	Check(ctx context.Context) []models.Requirement
}

// NetauditBaselines reads an automation's latest scan result. *persistence.WorkspaceManager satisfies it.
type NetauditBaselines interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
}

// NetauditHandlers serves the operator's network-audit settings and the latest findings.
type NetauditHandlers struct {
	scope     NetauditScope
	readiness NetauditReadiness
	baselines NetauditBaselines
}

func NewNetauditHandlers(scope NetauditScope, readiness NetauditReadiness, baselines NetauditBaselines) *NetauditHandlers {
	return &NetauditHandlers{scope: scope, readiness: readiness, baselines: baselines}
}

// findingView is a finding with whether the latest scan found it for the first time.
type findingView struct {
	models.ScanFinding
	New bool `json:"new"`
}

// findingsResponse is an automation's latest scan as a screen needs it.
type findingsResponse struct {
	Scanned   bool                 `json:"scanned"`
	ScannedAt string               `json:"scanned_at,omitempty"`
	Engine    string               `json:"engine,omitempty"`
	Findings  []findingView        `json:"findings"`
	Resolved  []models.ScanFinding `json:"resolved"`
}

// GetFindings returns the latest scan of one automation: its findings (marking the new ones) and what was resolved
// since the scan before. Scanned is false when the automation has not scanned yet.
func (h *NetauditHandlers) GetFindings(w http.ResponseWriter, r *http.Request) {
	workspaceID, automation := r.URL.Query().Get("workspace_id"), r.URL.Query().Get("automation")
	if workspaceID == "" || automation == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id and automation are required")
		return
	}
	b, err := h.baselines.ReadBaseline(workspaceID, automation)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := findingsResponse{Findings: []findingView{}, Resolved: []models.ScanFinding{}}
	if !b.ScannedAt.IsZero() {
		isNew := make(map[string]bool, len(b.NewKeys))
		for _, k := range b.NewKeys {
			isNew[k] = true
		}
		resp.Scanned, resp.ScannedAt, resp.Engine = true, b.ScannedAt.UTC().Format(time.RFC3339), b.Engine
		for _, f := range b.Findings {
			resp.Findings = append(resp.Findings, findingView{ScanFinding: f, New: isNew[f.Key()]})
		}
		if b.Resolved != nil {
			resp.Resolved = b.Resolved
		}
	}
	respondJSON(w, resp)
}

// GetReadiness lists every unmet prerequisite with the one step that would meet it. An empty list means scans can run.
func (h *NetauditHandlers) GetReadiness(w http.ResponseWriter, r *http.Request) {
	requirements := h.readiness.Check(r.Context())
	if requirements == nil {
		requirements = []models.Requirement{}
	}
	respondJSON(w, struct {
		Ready        bool                 `json:"ready"`
		Requirements []models.Requirement `json:"requirements"`
	}{Ready: len(requirements) == 0, Requirements: requirements})
}

// scopeResponse is the scan scope with the networks the machine is attached to, so the settings page can offer them.
type scopeResponse struct {
	netaudit.ScopeView
	Suggestions []netaudit.Suggestion `json:"suggestions"`
	// SuggestionsError says why the machine's networks could not be listed; a failure to list them never hides the
	// saved scope.
	SuggestionsError string `json:"suggestions_error,omitempty"`
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
	resp := scopeResponse{ScopeView: view, Suggestions: suggestions}
	if err != nil {
		resp.Suggestions, resp.SuggestionsError = nil, err.Error()
	}
	if resp.Suggestions == nil {
		resp.Suggestions = []netaudit.Suggestion{}
	}
	respondJSON(w, resp)
}

// isScopeRefusal reports whether err is the operator's input being refused rather than a failure to save.
func isScopeRefusal(err error) bool {
	return errors.Is(err, scanscope.ErrBadEntry) || errors.Is(err, scanscope.ErrNotScannable)
}
