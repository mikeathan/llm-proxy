package api

import (
	"net/http"
	"os"

	"llm-proxy/internal/logging"
	"llm-proxy/internal/workspace"
	"llm-proxy/models"
)

type WorkspaceHandlers struct {
	manager   *workspace.Manager
	scheduler *workspace.Scheduler
	logger    logging.Logger
}

func NewWorkspaceHandlers(mgr *workspace.Manager, sched *workspace.Scheduler, logger logging.Logger) *WorkspaceHandlers {
	return &WorkspaceHandlers{
		manager:   mgr,
		scheduler: sched,
		logger:    logger,
	}
}

func (h *WorkspaceHandlers) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	list, err := h.manager.ListWorkspaces()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list workspaces: "+err.Error())
		return
	}
	respondJSON(w, list)
}

func (h *WorkspaceHandlers) SaveWorkspace(w http.ResponseWriter, r *http.Request) {
	var req models.Workspace
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace ID is required")
		return
	}

	if err := h.manager.WriteConfig(req.ID, &req.Config); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	if err := h.manager.WriteHeartbeat(req.ID, req.Heartbeat); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save heartbeat: "+err.Error())
		return
	}

	cfg, _ := h.manager.ReadConfig(req.ID)
	state, _ := h.manager.ReadState(req.ID)
	hb, _ := h.manager.ReadHeartbeat(req.ID)

	respondJSON(w, &models.Workspace{
		ID:        req.ID,
		Config:    *cfg,
		State:     *state,
		Heartbeat: hb,
	})
}

func (h *WorkspaceHandlers) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "id query parameter is required")
		return
	}

	dir := h.manager.BaseDir() + "/" + id
	if err := os.RemoveAll(dir); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete workspace: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

func (h *WorkspaceHandlers) TriggerHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "id query parameter is required")
		return
	}

	if err := h.scheduler.ExecuteHeartbeat(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "heartbeat failed: "+err.Error())
		return
	}

	state, err := h.manager.ReadState(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read updated state: "+err.Error())
		return
	}

	respondJSON(w, state)
}
