package api

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
	"net/http"
)

type Dispatcher interface {
	Persistence() *persistence.WorkspaceManager
	Register(workspaceID string, auto *models.Automation) error
	Unregister(workspaceID string, automationName string) error
	ListAll() []*automation.AutomationEntry
	Trigger(ctx context.Context, workspaceID string, automationName string) error
	StopAutomation(workspaceID string) error
	Metrics() *automation.DispatcherMetrics
	Events() *automation.EventBus
	GlobalActivity() []models.AutomationRun
	UnregisterWorkspace(workspaceID string)
	ClearWorkspaceHistory(workspaceID string)
}

type DispatcherHandlers struct {
	dispatcher Dispatcher
}

// NewDispatcherHandlers creates new dispatcher handlers.
func NewDispatcherHandlers(d Dispatcher) *DispatcherHandlers {
	return &DispatcherHandlers{dispatcher: d}
}

type AutomationInfo struct {
	ID           string                 `json:"id"`
	Workspace    string                 `json:"workspace"`
	Name         string                 `json:"name"`
	TaskFile     string                 `json:"task_file"`
	Strategy     string                 `json:"strategy"`
	Trigger      string                 `json:"trigger"`
	TriggerValue string                 `json:"trigger_value,omitempty"`
	Model        string                 `json:"model,omitempty"`
	LastOutput   string                 `json:"last_output,omitempty"`
	LastError    string                 `json:"last_error,omitempty"`
	IsRunning    bool                   `json:"is_running"`
	History      []models.AutomationRun `json:"history,omitempty"`
}

func (h *DispatcherHandlers) ListAutomations(w http.ResponseWriter, r *http.Request) {
	entries := h.dispatcher.ListAll()
	infos := make([]AutomationInfo, 0, len(entries))

	for _, entry := range entries {
		info := AutomationInfo{
			ID:           entry.ID,
			Workspace:    entry.Workspace,
			Name:         entry.Name,
			Trigger:      string(entry.Trigger.Type()),
			TriggerValue: entry.Trigger.Value(),
			TaskFile:     entry.TaskFile,
			Strategy:     entry.Strategy.Name(),
			Model:        entry.Model,
		}

		// Try to fetch state for last output and history
		if state, err := h.dispatcher.Persistence().ReadState(entry.Workspace); err == nil {
			// Get specific history for this automation
			for _, run := range state.History {
				if run.AutomationName == entry.Name {
					info.History = append(info.History, run)
				}
			}

			// Use per-automation latest if available, otherwise fallback to global last
			if last, ok := state.LastRuns[entry.Name]; ok {
				info.LastOutput = last.Output
				info.LastError = last.Error
			} else {
				info.LastOutput = state.LastOutput
				info.LastError = state.LastError
			}
			info.IsRunning = state.IsRunning
		}

		infos = append(infos, info)
	}

	respondJSON(w, infos)
}

// TriggerAutomation manually triggers an automation by workspace ID and name.
func (h *DispatcherHandlers) TriggerAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
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

// StopAutomation stops any active automation for the specified workspace.
func (h *DispatcherHandlers) StopAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	if workspaceID == "" {
		respondError(w, http.StatusBadRequest, "workspace ID is required")
		return
	}

	if err := h.dispatcher.StopAutomation(workspaceID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, map[string]string{
		"status":    "stopped",
		"workspace": workspaceID,
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

func (h *DispatcherHandlers) GetWorkspaceState(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	state, err := h.dispatcher.Persistence().ReadState(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, state)
}

func (h *DispatcherHandlers) GetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	cfg, err := h.dispatcher.Persistence().ReadConfig(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, cfg)
}

func (h *DispatcherHandlers) UpdateWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	var cfg models.WorkspaceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	lock, err := h.dispatcher.Persistence().AcquireLock(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer h.dispatcher.Persistence().ReleaseLock(lock)

	if err := h.dispatcher.Persistence().WriteConfig(workspaceID, &cfg); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Re-register all automations to pick up new model/guardrail configs
	for _, auto := range cfg.Automations {
		h.dispatcher.Register(workspaceID, auto)
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *DispatcherHandlers) StreamWorkspaceEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	if workspaceID == "" {
		respondError(w, http.StatusBadRequest, "workspace is required")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Subscribe to events
	ch, recent := h.dispatcher.Events().Subscribe(workspaceID)
	defer h.dispatcher.Events().Unsubscribe(workspaceID, ch)

	// Context for cancellation
	ctx := r.Context()

	// Initial ping
	fmt.Fprintf(w, "event: ping\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Replay recent events for current run
	for _, ev := range recent {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: agent_update\ndata: %s\n\n", string(data))
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: agent_update\ndata: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}

func (h *DispatcherHandlers) GetGlobalActivity(w http.ResponseWriter, r *http.Request) {
	history := h.dispatcher.GlobalActivity()
	respondJSON(w, history)
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

	// Create default task files in root
	defaultTaskFiles := map[string]string{
		models.HeartbeatFilename:   assistant.DefaultHeartbeat,
		models.AgentPromptFilename: assistant.DefaultAgentPrompt,
		models.RulesFilename:       assistant.DefaultRules,
	}

	for filename, content := range defaultTaskFiles {
		if err := h.dispatcher.Persistence().WriteTaskFile(req.ID, filename, content); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create default file "+filename+": "+err.Error())
			return
		}
	}

	// Create initial config in .internal
	if err := h.dispatcher.Persistence().WriteConfig(req.ID, &models.WorkspaceConfig{
		Temperature: 0.7,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create config: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "created", "id": req.ID})
}

func (h *DispatcherHandlers) ListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	files, err := h.dispatcher.Persistence().ListFiles(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, files)
}

func (h *DispatcherHandlers) ReadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	filename := r.PathValue("file")

	content, err := h.dispatcher.Persistence().ReadTaskFile(workspaceID, filename)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, map[string]string{"content": content})
}

func (h *DispatcherHandlers) WriteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
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

func (h *DispatcherHandlers) UpdateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	automationName := r.PathValue("automation")

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

	found := false
	for i, a := range cfg.Automations {
		if a.Name == automationName {
			cfg.Automations[i] = &auto
			found = true
			break
		}
	}

	if !found {
		respondError(w, http.StatusNotFound, "automation not found")
		return
	}

	if err := h.dispatcher.Persistence().WriteConfig(workspaceID, cfg); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If name changed, unregister old name
	if automationName != auto.Name {
		h.dispatcher.Unregister(workspaceID, automationName)
	}

	// Re-register (or register new name)
	if err := h.dispatcher.Register(workspaceID, &auto); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *DispatcherHandlers) CreateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)

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
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	filename := r.PathValue("file")

	if err := h.dispatcher.Persistence().DeleteTaskFile(workspaceID, filename); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *DispatcherHandlers) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)

	// Clear from memory first
	h.dispatcher.UnregisterWorkspace(workspaceID)
	h.dispatcher.ClearWorkspaceHistory(workspaceID)

	if err := h.dispatcher.Persistence().DeleteWorkspace(workspaceID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}
func (h *DispatcherHandlers) DeleteAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue(models.WorkspaceIDParam)
	automationName := r.PathValue("automation")

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

	found := false
	for i, a := range cfg.Automations {
		if a.Name == automationName {
			cfg.Automations = append(cfg.Automations[:i], cfg.Automations[i+1:]...)
			found = true
			break
		}
	}

	// Always try to unregister from dispatcher (handles "ghost" entries)
	h.dispatcher.Unregister(workspaceID, automationName)

	if !found {
		// If not in config AND wasn't registered, then it's truly not found
		// But usually we just want to signal success if we cleaned up the dispatcher
		respondJSON(w, map[string]string{"status": "deleted/cleaned"})
		return
	}

	if err := h.dispatcher.Persistence().WriteConfig(workspaceID, cfg); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "deleted"})
}
