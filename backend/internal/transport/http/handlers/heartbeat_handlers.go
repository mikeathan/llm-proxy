package handlers

import (
	"net/http"

	"llm-proxy/models"
)

// GetHeartbeat returns the workspace heartbeat's settings, last check and the facts the panel warns about.
func (h *DispatcherHandlers) GetHeartbeat(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	h.respondHeartbeatState(w, workspaceID)
}

// PutHeartbeat saves the heartbeat settings into the workspace config and schedules (or unschedules) it at once,
// without waiting for the config watcher.
func (h *DispatcherHandlers) PutHeartbeat(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.parse(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	var cfg models.HeartbeatConfig
	if !decodeJSONBody(w, r, &cfg) {
		return
	}
	if err := cfg.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateRunOptions(cfg.Automation()); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.workspace.MutateConfig(workspaceID, func(existing *models.WorkspaceConfig) error {
		existing.Heartbeat = &cfg
		return nil
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.applyHeartbeatSchedule(workspaceID, cfg)
	h.respondHeartbeatState(w, workspaceID)
}

func (h *DispatcherHandlers) applyHeartbeatSchedule(workspaceID string, cfg models.HeartbeatConfig) {
	if cfg.Enabled {
		if err := h.dispatcher.Register(workspaceID, cfg.Automation()); err != nil {
			h.logger.Error("failed to schedule heartbeat", "workspace", workspaceID, "error", err)
		}
		return
	}
	// Nothing registered is the normal case for a heartbeat that was never on.
	_ = h.dispatcher.Unregister(workspaceID, models.HeartbeatAutomationName)
}

func (h *DispatcherHandlers) respondHeartbeatState(w http.ResponseWriter, workspaceID string) {
	state, err := h.dispatcher.HeartbeatState(workspaceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, state)
}
