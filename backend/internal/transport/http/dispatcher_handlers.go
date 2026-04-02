package api

import (
	"encoding/json"
	"llm-proxy/internal/core/automation"
	"llm-proxy/models"
	"net/http"
)

type DispatcherHandlers struct {
	dispatcher *automation.Dispatcher
}

// NewDispatcherHandlers creates new dispatcher handlers.
func NewDispatcherHandlers(d *automation.Dispatcher) *DispatcherHandlers {
	return &DispatcherHandlers{dispatcher: d}
}

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

func (h *DispatcherHandlers) GetDispatcherMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.dispatcher.Metrics()

	respondJSON(w, map[string]interface{}{
		"total_executions": metrics.TotalExecutions,
		"successful":       metrics.SuccessfulExecutions,
		"failed":           metrics.FailedExecutions,
		"skipped":          metrics.SkippedExecutions,
		"total_latency_ms": metrics.TotalLatency.Milliseconds(),
	})
}

type AutomationInfo struct {
	ID        string             `json:"id"`
	Workspace string             `json:"workspace"`
	Name      string             `json:"name"`
	TaskFile  string             `json:"task_file"`
	Strategy  string             `json:"strategy"`
	Trigger   models.TriggerType `json:"trigger"`
}

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (h *DispatcherHandlers) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.dispatcher.Persistence().ListWorkspaces()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(workspaces))
	for _, ws := range workspaces {
		result = append(result, map[string]interface{}{
			"id": ws.ID,
		})
	}
	respondJSON(w, result)
}

// CreateWorkspace creates a new workspace.
func (h *DispatcherHandlers) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.ID == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	// Just try to acquire a lock to create the directory
	lock, err := h.dispatcher.Persistence().AcquireLock(req.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer h.dispatcher.Persistence().ReleaseLock(lock)

	// Create default files
	defaultFiles := map[string]string{
		"heartbeat.md": "# Heartbeat\n\nWorkspace initialized.",
		"config.yaml":  "model: gpt-4o\ntemperature: 0.7\nautomations: []",
	}

	for filename, content := range defaultFiles {
		if err := h.dispatcher.Persistence().WriteTaskFile(req.ID, filename, content); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create default file "+filename+": "+err.Error())
			return
		}
	}

	respondJSON(w, map[string]string{"status": "created", "id": req.ID})
}

func (h *DispatcherHandlers) ListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	files, err := h.dispatcher.Persistence().ListFiles(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, files)
}

func (h *DispatcherHandlers) ReadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	filename := r.PathValue("file")

	content, err := h.dispatcher.Persistence().ReadTaskFile(workspaceID, filename)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, map[string]string{"content": content})
}

func (h *DispatcherHandlers) WriteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	filename := r.PathValue("file")

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.dispatcher.Persistence().WriteTaskFile(workspaceID, filename, req.Content); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "saved"})
}

func (h *DispatcherHandlers) CreateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")

	var auto models.Automation
	if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	lock, err := h.dispatcher.Persistence().AcquireLock(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer h.dispatcher.Persistence().ReleaseLock(lock)

	cfg, err := h.dispatcher.Persistence().ReadConfig(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cfg.Automations = append(cfg.Automations, &auto)

	if err := h.dispatcher.Persistence().WriteConfig(workspaceID, cfg); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.dispatcher.Register(workspaceID, &auto); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "created"})
}

func (h *DispatcherHandlers) DeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	filename := r.PathValue("file")

	if err := h.dispatcher.Persistence().DeleteTaskFile(workspaceID, filename); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *DispatcherHandlers) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")

	if err := h.dispatcher.Persistence().DeleteWorkspace(workspaceID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}
