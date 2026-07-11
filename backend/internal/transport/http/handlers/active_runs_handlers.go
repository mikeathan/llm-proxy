package handlers

import (
	"net/http"

	"llm-proxy/models"
)

// ActiveRunsResponse aggregates the authoritative "currently executing" state
// for a workspace. It is the single ground-truth source the frontend polls to
// drive "running" notifications (assistant glow, automation indicators, future
// surfaces) instead of trusting sticky client-side flags that can miss a
// completion event.
type ActiveRunsResponse struct {
	AssistantRunning  bool `json:"assistant_running"`
	AutomationRunning bool `json:"automation_running"`
}

// ActiveRunsHandler reports what is executing per workspace by delegating to the
// authoritative running-state sources of each subsystem. Dependencies are passed
// as funcs so the handler stays decoupled and trivially testable.
type ActiveRunsHandler struct {
	assistantRunning  func(workspaceID string) bool
	automationRunning func(workspaceID string) bool
}

// NewActiveRunsHandler wires the handler to the subsystem-specific running
// checks. Both funcs are read-only and must be safe for concurrent use.
func NewActiveRunsHandler(
	assistantRunning func(workspaceID string) bool,
	automationRunning func(workspaceID string) bool,
) *ActiveRunsHandler {
	return &ActiveRunsHandler{
		assistantRunning:  assistantRunning,
		automationRunning: automationRunning,
	}
}

// ServeHTTP returns the aggregated running state for the requested workspace.
func (h *ActiveRunsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}

	respondJSON(w, ActiveRunsResponse{
		AssistantRunning:  h.assistantRunning(workspaceID),
		AutomationRunning: h.automationRunning(workspaceID),
	})
}
