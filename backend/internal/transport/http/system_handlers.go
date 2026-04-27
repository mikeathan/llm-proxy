package api

import (
	"net/http"
	"os"
	"time"

	"llm-proxy/models"
)

// AdminVersionHandler handles GET /admin/api/version
func (h *AdminHandlers) AdminVersionHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{
		"version":    h.buildInfo.Version,
		"commit":     h.buildInfo.Commit,
		"build_date": h.buildInfo.BuildDate,
	})
}

func (h *AdminHandlers) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	sys := h.admin.GetSystem()
	reg := h.admin.GetRegistry()
	settings := h.admin.GetSettings()
	id, secret := h.admin.ServiceCredentials()
	cfg := adminConfigView{
		WorkspacesDir:       sys.WorkspacesDir,
		ModelHost:           sys.Server.ModelHost,
		IdleTimeoutSecs:     sys.Server.IdleTimeoutSecs,
		GPUProvider:         h.admin.GPUConfig().Provider,
		GPUBinary:           h.admin.GPUConfig().Binary,
		GPUIndex:            h.admin.GPUConfig().Index,
		DefaultArgs:         settings.Local.DefaultArgs,
		ServiceClientID:     id,
		ServiceClientSecret: secret,
		PrimaryModel:        reg.PrimaryModel,
		FallbackModel:       reg.FallbackModel,
		Providers:           h.getProvidersView(),
		Guardrails:          h.admin.GetGuardrails(),
		Communication:       reg.Communication,
		Search:              reg.Search,
	}
	respondJSON(w, cfg)
}

// AdminSystemHandler handles GET /admin/api/system
func (h *AdminHandlers) AdminSystemHandler(w http.ResponseWriter, r *http.Request) {
	sys := h.admin.GetSystem()
	settings := h.admin.GetSettings()
	view := adminSystemView{
		Bind:            sys.Server.Bind,
		ModelHost:       sys.Server.ModelHost,
		IdleTimeoutSecs: sys.Server.IdleTimeoutSecs,
		WorkspacesDir:   sys.WorkspacesDir,
		GPU:             h.admin.GPUConfig(),
		Environment:     sys.Server.Environment,
		Local:           settings.Local,
	}
	respondJSON(w, view)
}

// AdminSystemPutHandler handles PUT /admin/api/system
func (h *AdminHandlers) AdminSystemPutHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SystemUpdatePayload
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.admin.ApplySystemUpdate(r.Context(), req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update config: "+err.Error())
		return
	}

	respondJSON(w, h.admin.GetSystem())
}

func (h *AdminHandlers) AdminConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SystemUpdatePayload
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err2 := h.admin.ApplySystemUpdate(r.Context(), req); err2 != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update config: "+err2.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminRestartHandler handles POST /admin/api/system/restart
func (h *AdminHandlers) AdminRestartHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Restart requested via Admin UI")
	respondJSON(w, map[string]string{"status": "restarting", "message": "Backend is restarting..."})

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

// AdminHostSettingsHandler handles GET /admin/api/host
func (h *AdminHandlers) AdminHostSettingsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.HostSettings())
}

// AdminHostSettingsPutHandler handles PUT /admin/api/host
func (h *AdminHandlers) AdminHostSettingsPutHandler(w http.ResponseWriter, r *http.Request) {
	var req models.HostSettings
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.admin.UpdateHostSettings(req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update host settings: "+err.Error())
		return
	}

	respondJSON(w, req)
}

// AdminTerminalResetHandler handles POST /admin/api/host/terminal/reset
func (h *AdminHandlers) AdminTerminalResetHandler(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspaceID")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspaceID query parameter is required")
		return
	}

	if err := h.admin.ResetShell(workspaceID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to reset terminal session: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "ok", "message": "Terminal session reset triggered for " + workspaceID})
}

// AdminTerminalSessionsHandler handles GET /admin/api/host/terminal/sessions
func (h *AdminHandlers) AdminTerminalSessionsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.ListShellSessions())
}



