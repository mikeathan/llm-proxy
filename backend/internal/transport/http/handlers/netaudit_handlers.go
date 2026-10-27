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
	SetDeviceTimeout(minutes int) error
	Suggestions() ([]netaudit.Suggestion, error)
	CheckTargets(ctx context.Context, targets []string) []netaudit.TargetProblem
}

// NetauditReadiness answers what is missing before scans can run.
type NetauditReadiness interface {
	Check(ctx context.Context) []models.Requirement
}

// NetauditBaselines reads an automation's latest scan result. *persistence.WorkspaceManager satisfies it.
type NetauditBaselines interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
}

// NetauditAcks is what the operator can do with findings: accept one for a while, take the acceptance back, list them.
type NetauditAcks interface {
	List(workspaceID string) ([]models.ScanAck, error)
	Accept(workspaceID, automation, key, note string, days int) (models.ScanAck, error)
	Revoke(workspaceID, key string) (bool, error)
}

// NetauditHandlers serves the operator's network-audit settings, the latest findings and the accepted findings.
type NetauditHandlers struct {
	scope     NetauditScope
	readiness NetauditReadiness
	baselines NetauditBaselines
	acks      NetauditAcks
}

func NewNetauditHandlers(scope NetauditScope, readiness NetauditReadiness, baselines NetauditBaselines, acks NetauditAcks) *NetauditHandlers {
	return &NetauditHandlers{scope: scope, readiness: readiness, baselines: baselines, acks: acks}
}

// findingView is a finding with whether the latest scan found it for the first time, and whether the operator has
// accepted it (and until when).
type findingView struct {
	models.ScanFinding
	// Key identifies the finding for accepting it; the page never builds one itself.
	Key string `json:"key"`
	New bool   `json:"new"`
	// Fix is the suggested remedy for a check finding, fixed text the operator applies by hand; empty for an open port.
	Fix string `json:"fix,omitempty"`
	// Changed is true when the version differs from the scan before; PreviousVersion is what it was (possibly empty).
	Changed         bool   `json:"changed"`
	PreviousVersion string `json:"previous_version,omitempty"`
	Accepted        bool   `json:"accepted"`
	AcceptedNote    string `json:"accepted_note,omitempty"`
	AcceptedTill    string `json:"accepted_until,omitempty"`
}

// findingsResponse is an automation's latest scan as a screen needs it.
type findingsResponse struct {
	Scanned   bool                 `json:"scanned"`
	ScannedAt string               `json:"scanned_at,omitempty"`
	Engine    string               `json:"engine,omitempty"`
	Findings  []findingView        `json:"findings"`
	Resolved  []models.ScanFinding `json:"resolved"`
	// Unscanned are the devices the engine ran out of time on; DevicesUp is how many addresses answered.
	Unscanned []string `json:"unscanned"`
	DevicesUp int      `json:"devices_up"`
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
	resp := findingsResponse{Findings: []findingView{}, Resolved: []models.ScanFinding{}, Unscanned: []string{}}
	if !b.ScannedAt.IsZero() {
		isNew := make(map[string]bool, len(b.NewKeys))
		for _, k := range b.NewKeys {
			isNew[k] = true
		}
		resp.Scanned, resp.ScannedAt, resp.Engine, resp.DevicesUp = true, b.ScannedAt.UTC().Format(time.RFC3339), b.Engine, b.DevicesUp
		resp.Unscanned = append(resp.Unscanned, b.Unscanned...)
		acks := h.activeAcks(workspaceID)
		for _, f := range b.Findings {
			view := findingView{ScanFinding: f, Key: f.Key(), New: isNew[f.Key()], Fix: netaudit.RemediationFor(f.Check)}
			view.PreviousVersion, view.Changed = b.ChangedFrom[f.Key()]
			if a, ok := acks[f.Key()]; ok && netaudit.StateFor(a, f, time.Now()) == netaudit.AckActive {
				view.Accepted, view.AcceptedNote, view.AcceptedTill = true, a.Note, a.ExpiresAt.UTC().Format(time.RFC3339)
			}
			resp.Findings = append(resp.Findings, view)
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
	// DeviceTimeoutMinutes is the per-device time limit; one of netaudit.AllowedDeviceTimeouts.
	DeviceTimeoutMinutes *int `json:"device_timeout_minutes"`
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
	if req.DeviceTimeoutMinutes != nil {
		if err := h.scope.SetDeviceTimeout(*req.DeviceTimeoutMinutes); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, netaudit.ErrBadDeviceTimeout) {
				status = http.StatusBadRequest
			}
			writeJSONError(w, status, err.Error())
			return
		}
		view = h.scope.View()
	}
	h.respondScope(w, view)
}

// checkTargetsRequest is the targets of an audit as it is being set up.
type checkTargetsRequest struct {
	Targets []string `json:"targets"`
}

// CheckTargets says before a run which of an audit's targets it would refuse, and why, so the form can warn instead of
// the run failing at night.
func (h *NetauditHandlers) CheckTargets(w http.ResponseWriter, r *http.Request) {
	var req checkTargetsRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	problems := h.scope.CheckTargets(r.Context(), req.Targets)
	if problems == nil {
		problems = []netaudit.TargetProblem{}
	}
	respondJSON(w, struct {
		Problems []netaudit.TargetProblem `json:"problems"`
	}{problems})
}

// activeAcks indexes the workspace's acceptances by finding key; a failure to read them lists every finding as not accepted.
func (h *NetauditHandlers) activeAcks(workspaceID string) map[string]models.ScanAck {
	out := map[string]models.ScanAck{}
	if h.acks == nil {
		return out
	}
	acks, err := h.acks.List(workspaceID)
	if err != nil {
		return out
	}
	for _, a := range acks {
		out[a.Key] = a
	}
	return out
}

// ackRequest accepts one finding of an automation's latest scan.
type ackRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Automation  string `json:"automation"`
	Key         string `json:"key"`
	Note        string `json:"note"`
	Days        int    `json:"days"`
}

// ListAcks returns the workspace's accepted findings, including expired ones until they are dropped.
func (h *NetauditHandlers) ListAcks(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	acks, err := h.acks.List(workspaceID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acks == nil {
		acks = []models.ScanAck{}
	}
	respondJSON(w, acks)
}

// AcceptFinding accepts a finding of the automation's latest scan for a number of days (90 when not given).
func (h *NetauditHandlers) AcceptFinding(w http.ResponseWriter, r *http.Request) {
	var req ackRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.WorkspaceID == "" || req.Automation == "" || req.Key == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id, automation and key are required")
		return
	}
	ack, err := h.acks.Accept(req.WorkspaceID, req.Automation, req.Key, req.Note, req.Days)
	switch {
	case errors.Is(err, netaudit.ErrBadAck):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, netaudit.ErrNoSuchFinding):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	default:
		respondJSON(w, ack)
	}
}

// RevokeAck takes an acceptance back.
func (h *NetauditHandlers) RevokeAck(w http.ResponseWriter, r *http.Request) {
	workspaceID, key := r.URL.Query().Get("workspace_id"), r.URL.Query().Get("key")
	if workspaceID == "" || key == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id and key are required")
		return
	}
	ok, err := h.acks.Revoke(workspaceID, key)
	switch {
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	case !ok:
		writeJSONError(w, http.StatusNotFound, "no accepted finding with that key")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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
