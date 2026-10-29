package handlers

import (
	"errors"
	"net/http"
	"os"
	"time"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/logging"
	api "llm-proxy/internal/transport/http"
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
	id, _ := h.admin.ServiceCredentials()
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
		// ServiceClientSecret is deliberately never emitted: the frontend does
		// not need it (only the ID is shown; a new value is set via PUT) and
		// returning it verbatim leaked the credential to any client.
		PrimaryModel:  reg.PrimaryModel,
		FallbackModel: reg.FallbackModel,
		Providers:     getProvidersView(h.admin),
		Guardrails:    h.admin.GetGuardrails(),
		Communication: reg.Communication.MaskedWebhookSecrets(),
		Search:        reg.Search,
		RunLogging:    &models.RunLoggingConfig{Enabled: h.admin.RunLoggingEnabled()},
		Memory:        settings.Memory.HotDefaults(),
		Reminders:     settings.Reminders.Effective(),
		Agent:         settings.Agent.Effective(),
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

	if !h.stepUpOK(w, r, req) {
		return
	}
	if err := h.admin.ApplySystemUpdate(r.Context(), req); err != nil {
		writeSystemUpdateError(w, err)
		return
	}

	respondJSON(w, h.admin.GetSystem())
}

func (h *SystemHandlers) AdminConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SystemUpdatePayload
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if !h.stepUpOK(w, r, req) {
		return
	}
	if err := h.admin.ApplySystemUpdate(r.Context(), req); err != nil {
		writeSystemUpdateError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// stepUpOK lets an update through, unless it changes a program the server runs (or its arguments or environment) and the
// request has not just confirmed the passphrase. Those are command execution as the service user. It answers 403 itself.
func (h *SystemHandlers) stepUpOK(w http.ResponseWriter, r *http.Request, req models.SystemUpdatePayload) bool {
	if api.StepUpGranted(r.Context()) || !h.admin.SystemUpdateChangesWhatRuns(req) {
		return true
	}
	api.RefuseStepUp(w, r)
	return false
}

// writeSystemUpdateError answers a failed system update: a bad model name or an invalid search, scheduler, agent or
// reminders block is the caller's mistake (400); anything else is a server error (500).
func writeSystemUpdateError(w http.ResponseWriter, err error) {
	var notFound *models.ModelNotFoundError
	switch {
	case errors.As(err, &notFound):
		writeJSONError(w, http.StatusBadRequest, notFound.Error())
	case models.IsSearchConfigError(err) || models.IsSchedulerConfigError(err) || models.IsAgentConfigError(err) || models.IsRemindersConfigError(err):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "failed to update config: "+err.Error())
	}
}

// AdminRestartHandler handles POST /admin/api/system/restart
func (h *SystemHandlers) AdminRestartHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Restart requested via Admin UI")
	respondJSON(w, map[string]string{"status": "restarting", "message": "Backend is restarting..."})

	go shutdownAfterResponse(exitCodeRestart)
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

// AdminFactoryResetHandler handles POST /admin/api/system/factory-reset
func (h *SystemHandlers) AdminFactoryResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	res, err := h.admin.FactoryReset()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "factory reset failed: "+err.Error())
		return
	}
	respondJSON(w, res)
}

// AdminClearRuntimeDataHandler handles POST /admin/api/system/clear-runtime-data
func (h *SystemHandlers) AdminClearRuntimeDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := h.admin.ClearRuntimeData(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "clear runtime data failed: "+err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

// AdminWipeoutHandler handles POST /admin/api/system/wipeout — a full uninstall
// that removes the data root, workspaces, and secrets, then stops the process.
func (h *SystemHandlers) AdminWipeoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	res, err := h.admin.Wipeout()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "wipeout failed: "+err.Error())
		return
	}
	respondJSON(w, res)

	// The data the process depends on is gone; stop it after the response flushes.
	go shutdownAfterResponse(exitCodeStop)
}

const (
	// exitCodeStop is a clean, intentional stop: supervisors (systemd
	// Restart=on-failure, launchd KeepAlive SuccessfulExit=false) leave it down.
	exitCodeStop = 0
	// exitCodeRestart is EX_TEMPFAIL. The process has no way to re-exec itself
	// under a service manager, so a restart exits non-zero and relies on the
	// supervisor to relaunch it — exit 0 would be treated as a deliberate stop
	// and the service would never come back.
	exitCodeRestart = 75
)

// shutdownAfterResponse waits briefly for the HTTP response to flush, then exits
// the process with code. It is a package variable so tests can stub the process
// exit (a real os.Exit would terminate the test binary).
var shutdownAfterResponse = func(code int) {
	time.Sleep(500 * time.Millisecond)
	os.Exit(code)
}
