package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
	"net/http"
	"regexp"
	"strings"
)

var validIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validateID(id string) bool {
	return validIDRegex.MatchString(id)
}

type Dispatcher interface {
	Persistence() *persistence.WorkspaceManager
	Register(workspaceID string, auto *models.Automation) error
	Unregister(workspaceID string, automationName string) error
	ListAll() []*automation.AutomationEntry
	Trigger(ctx context.Context, workspaceID string, automationName string, recordingRef string) error
	StopAutomation(workspaceID string) error
	Metrics() *automation.DispatcherMetrics
	Events() *automation.EventBus
	GlobalActivity() []models.AutomationRun
	UnregisterWorkspace(workspaceID string)
	ClearWorkspaceHistory(workspaceID string)
}

type DispatcherHandlers struct {
	dispatcher Dispatcher
	workspace  WorkspaceService
	logger     logging.Logger
}

func NewDispatcherHandlers(d Dispatcher, ws WorkspaceService, logger logging.Logger) *DispatcherHandlers {
	return &DispatcherHandlers{dispatcher: d, workspace: ws, logger: logger}
}

func (h *DispatcherHandlers) parse(w http.ResponseWriter, r *http.Request, keys ...string) (wsID string, autoName string, ok bool) {
	for _, k := range keys {
		val := r.PathValue(k)
		if val == "" {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("%s is required", k))
			return "", "", false
		}
		if !validateID(val) {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", k))
			return "", "", false
		}

		if k == models.WorkspaceIDParam {
			wsID = val
		}
		if k == "automation" {
			autoName = val
		}
	}
	return wsID, autoName, true
}

func (h *DispatcherHandlers) validateAutomation(auto *models.Automation) error {
	if auto == nil {
		return fmt.Errorf("automation data is required")
	}
	if !validateID(auto.Name) {
		return fmt.Errorf("invalid automation name in payload")
	}
	return nil
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
	RecordingRef string                 `json:"recording_ref,omitempty"`
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
			RecordingRef: entry.RecordingRef,
		}

		if state, err := h.workspace.GetState(entry.Workspace); err == nil {
			for _, run := range state.History {
				if run.AutomationName == entry.Name {
					info.History = append(info.History, run)
				}
			}

			if last, ok := state.LastRuns[entry.Name]; ok {
				info.LastOutput = last.Output
				info.LastError = last.Error
			} else {
				info.LastOutput = state.LastOutput
				info.LastError = state.LastError
			}
			info.IsRunning = state.ActiveAutomation == entry.Name
		}

		infos = append(infos, info)
	}

	respondJSON(w, infos)
}

func (h *DispatcherHandlers) TriggerAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	state, err := h.workspace.GetState(workspaceID)
	if err == nil && state.IsRunning() {
		respondError(w, http.StatusConflict, fmt.Sprintf("automation '%s' is already running in workspace '%s'", state.ActiveAutomation, workspaceID))
		return
	}

	recordingRef := r.URL.Query().Get("recording_ref")

	go func() {
		if err := h.dispatcher.Trigger(context.Background(), workspaceID, automationName, recordingRef); err != nil {
			h.logger.Error("Async automation trigger failed",
				"workspace", workspaceID,
				"automation", automationName,
				"recording_ref", recordingRef,
				"error", err)
		}
	}()

	respondJSON(w, map[string]string{
		"status":     "triggered",
		"workspace":  workspaceID,
		"automation": automationName,
	})
}

func (h *DispatcherHandlers) StopAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
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
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	state, err := h.workspace.GetState(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, struct {
		*models.AgentState
		IsRunning bool `json:"is_running"`
	}{
		AgentState: state,
		IsRunning:  state.IsRunning(),
	})
}

func (h *DispatcherHandlers) GetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	cfg, err := h.workspace.GetConfig(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, cfg)
}

func (h *DispatcherHandlers) UpdateWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	var cfg models.WorkspaceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Acquire lock, read existing, merge, write, release — handled atomically by WorkspaceService.
	if err := h.workspace.MutateConfig(workspaceID, func(existing *models.WorkspaceConfig) {
		// Merge logic: If automations aren't in the request, keep the old ones
		if cfg.Automations == nil {
			cfg.Automations = existing.Automations
		}
		*existing = cfg
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, auto := range cfg.Automations {
		h.dispatcher.Register(workspaceID, auto)
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *DispatcherHandlers) StreamWorkspaceEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
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
	workspaces, err := h.workspace.ListWorkspaces()
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

	if !validateID(req.ID) {
		respondError(w, http.StatusBadRequest, "invalid workspace ID (must be alphanumeric/hyphen)")
		return
	}

	// Create default task files in workspace root
	defaultTaskFiles := map[string]string{
		models.HeartbeatFilename:   prompts.DefaultHeartbeat,
		models.AgentPromptFilename: prompts.DefaultAgentPrompt,
		models.RulesFilename:       prompts.DefaultRules,
	}

	if err := h.workspace.CreateWorkspace(req.ID, &models.WorkspaceConfig{
		Temperature: 0.7,
	}, defaultTaskFiles); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "created", "id": req.ID})
}

func (h *DispatcherHandlers) ListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	files, err := h.workspace.ListFiles(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, files)
}

func (h *DispatcherHandlers) ReadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	filename := r.PathValue("file")

	content, err := h.workspace.ReadTaskFile(workspaceID, filename)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, map[string]string{"content": content})
}

func (h *DispatcherHandlers) WriteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	filename := r.PathValue("file")

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.workspace.WriteTaskFile(workspaceID, filename, req.Content); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "saved"})
}

func (h *DispatcherHandlers) UpdateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	var auto models.Automation
	if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.validateAutomation(&auto); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var found bool
	if err := h.workspace.MutateConfig(workspaceID, func(cfg *models.WorkspaceConfig) {
		for i, a := range cfg.Automations {
			if strings.TrimSpace(a.Name) == automationName {
				newAuto := auto
				cfg.Automations[i] = &newAuto
				found = true
				return
			}
		}

		var names []string
		for _, a := range cfg.Automations {
			names = append(names, fmt.Sprintf("'%s'", a.Name))
		}
		h.logger.Warn("UpdateAutomation failed: automation not found",
			"workspace", workspaceID,
			"target", automationName,
			"available", strings.Join(names, ", "))
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !found {
		respondError(w, http.StatusNotFound, "automation not found")
		return
	}

	if automationName != auto.Name {
		h.dispatcher.Unregister(workspaceID, automationName)
	}

	regAuto := auto
	if err := h.dispatcher.Register(workspaceID, &regAuto); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *DispatcherHandlers) CreateAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	var auto models.Automation
	if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.validateAutomation(&auto); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.workspace.MutateConfig(workspaceID, func(cfg *models.WorkspaceConfig) {
		// Ensure no duplicate name
		for _, a := range cfg.Automations {
			if strings.TrimSpace(a.Name) == strings.TrimSpace(auto.Name) {
				respondError(w, http.StatusConflict, "automation already exists")
				return
			}
		}
		// Create a persistent copy in config
		newAuto := auto
		cfg.Automations = append(cfg.Automations, &newAuto)
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Register in memory
	regAuto := auto
	if err := h.dispatcher.Register(workspaceID, &regAuto); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "created"})
}

func (h *DispatcherHandlers) DeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	filename := r.PathValue("file")

	if err := h.workspace.DeleteTaskFile(workspaceID, filename); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *DispatcherHandlers) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}

	// Clear from memory first
	h.dispatcher.UnregisterWorkspace(workspaceID)
	h.dispatcher.ClearWorkspaceHistory(workspaceID)

	if err := h.workspace.DeleteWorkspace(workspaceID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *DispatcherHandlers) DeleteAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, automationName, ok := h.parse(w, r, models.WorkspaceIDParam, "automation")
	if !ok {
		return
	}

	var found bool
	if err := h.workspace.MutateConfig(workspaceID, func(cfg *models.WorkspaceConfig) {
		for i, a := range cfg.Automations {
			if a.Name == automationName {
				cfg.Automations = append(cfg.Automations[:i], cfg.Automations[i+1:]...)
				found = true
				break
			}
		}
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Always try to unregister from dispatcher (handles "ghost" entries)
	h.dispatcher.Unregister(workspaceID, automationName)

	if !found {
		respondJSON(w, map[string]string{"status": "deleted/cleaned"})
		return
	}

	respondJSON(w, map[string]string{"status": "deleted"})
}
