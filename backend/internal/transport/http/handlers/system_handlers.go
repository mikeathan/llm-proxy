package handlers

import (
	"net/http"
	"os"
	"time"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// SystemHandlers serves system-level admin endpoints.
type SystemHandlers struct {
	admin     AdminService
	logger    logging.Logger
	buildInfo *buildinfo.Info
}

func NewSystemHandlers(admin AdminService, logger logging.Logger, buildInfo *buildinfo.Info) *SystemHandlers {
	return &SystemHandlers{admin: admin, logger: logger, buildInfo: buildInfo}
}

// AdminVersionHandler handles GET /admin/api/version
func (h *SystemHandlers) AdminVersionHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{
		"version":    h.buildInfo.Version,
		"commit":     h.buildInfo.Commit,
		"build_date": h.buildInfo.BuildDate,
	})
}

func (h *SystemHandlers) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	sys := h.admin.GetSystem()
	reg := h.admin.GetRegistry()
	settings := h.admin.GetSettings()
	id, secret := h.admin.ServiceCredentials()
	cfg := adminConfigView{
		WorkspacesDir:        sys.WorkspacesDir,
		ModelHost:            sys.Server.ModelHost,
		IdleTimeoutSecs:      sys.Server.IdleTimeoutSecs,
		GPUProvider:          h.admin.GPUConfig().Provider,
		GPUBinary:            h.admin.GPUConfig().Binary,
		GPUIndex:             h.admin.GPUConfig().Index,
		GPUSampleIntervalSec: sys.Metrics.GPUSampleIntervalSec,
		GPUSmoothingAlpha:    sys.Metrics.GPUSmoothingAlpha,
		DefaultArgs:          settings.Local.DefaultArgs,
		ServiceClientID:      id,
		ServiceClientSecret:  secret,
		PrimaryModel:         reg.PrimaryModel,
		FallbackModel:        reg.FallbackModel,
		Providers:            getProvidersView(h.admin),
		Guardrails:           h.admin.GetGuardrails(),
		Communication:        reg.Communication,
		Search:               reg.Search,
		RunLogging:           &models.RunLoggingConfig{Enabled: h.admin.RunLoggingEnabled()},
	}
	respondJSON(w, cfg)
}

// AdminSystemHandler handles GET /admin/api/system
func (h *SystemHandlers) AdminSystemHandler(w http.ResponseWriter, r *http.Request) {
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
func (h *SystemHandlers) AdminSystemPutHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *SystemHandlers) AdminConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
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
func (h *SystemHandlers) AdminRestartHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Restart requested via Admin UI")
	respondJSON(w, map[string]string{"status": "restarting", "message": "Backend is restarting..."})

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

// AdminHostSettingsHandler handles GET /admin/api/host
func (h *SystemHandlers) AdminHostSettingsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.HostSettings())
}

// AdminHostSettingsPutHandler handles PUT /admin/api/host
func (h *SystemHandlers) AdminHostSettingsPutHandler(w http.ResponseWriter, r *http.Request) {
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
func (h *SystemHandlers) AdminTerminalResetHandler(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireQueryParam(w, r, "workspaceID")
	if !ok {
		return
	}

	if err := h.admin.ResetShell(workspaceID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to reset terminal session: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "ok", "message": "Terminal session reset triggered for " + workspaceID})
}

// AdminTerminalSessionsHandler handles GET /admin/api/host/terminal/sessions
func (h *SystemHandlers) AdminTerminalSessionsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, h.admin.ListShellSessions())
}
