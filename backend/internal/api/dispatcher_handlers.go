package api

import (
	"encoding/json"
	"net/http"

	"llm-proxy/internal/dispatcher"
)

// DispatcherHandlers handles REST API endpoints for the dispatcher.
type DispatcherHandlers struct {
	dispatcher *dispatcher.Dispatcher
}

// NewDispatcherHandlers creates new dispatcher handlers.
func NewDispatcherHandlers(d *dispatcher.Dispatcher) *DispatcherHandlers {
	return &DispatcherHandlers{dispatcher: d}
}

// ListAutomations returns all registered automations across all workspaces.
func (h *DispatcherHandlers) ListAutomations(w http.ResponseWriter, r *http.Request) {
	automations := h.dispatcher.ListAll()

	result := make([]AutomationInfo, 0, len(automations))
	for _, a := range automations {
		result = append(result, AutomationInfo{
			ID:        a.ID,
			Workspace: a.Workspace,
			Name:      a.Name,
			TaskFile:  a.TaskFile,
			Strategy:  a.Strategy.Name(),
			Trigger:   a.Trigger.Type(),
		})
	}

	respondJSON(w, result)
}

// TriggerAutomation manually triggers an automation by workspace ID and name.
func (h *DispatcherHandlers) TriggerAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	automationName := r.PathValue("automation")

	if workspaceID == "" || automationName == "" {
		respondError(w, http.StatusBadRequest, "workspace and automation name are required")
		return
	}

	if err := h.dispatcher.Trigger(r.Context(), workspaceID, automationName); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{
		"status":     "triggered",
		"workspace":  workspaceID,
		"automation": automationName,
	})
}

// GetDispatcherMetrics returns current dispatcher metrics.
func (h *DispatcherHandlers) GetDispatcherMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.dispatcher.Metrics()

	respondJSON(w, map[string]interface{}{
		"total_executions":   metrics.TotalExecutions,
		"successful":         metrics.SuccessfulExecutions,
		"failed":             metrics.FailedExecutions,
		"skipped":            metrics.SkippedExecutions,
		"total_latency_ms":   metrics.TotalLatency.Milliseconds(),
	})
}

// AutomationInfo represents an automation in API responses.
type AutomationInfo struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	TaskFile  string `json:"task_file"`
	Strategy  string `json:"strategy"`
	Trigger   string `json:"trigger"`
}

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
