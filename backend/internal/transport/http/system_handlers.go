package api

import (
	"net/http"
	"os"
	"time"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

func (h *AdminHandlers) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	sys := h.admin.GetSystem()
	reg := h.admin.GetRegistry()
	id, secret := h.admin.ServiceCredentials()
	cfg := adminConfigView{
		WorkspacesDir:       sys.WorkspacesDir,
		ModelHost:           sys.Server.ModelHost,
		IdleTimeoutSecs:     sys.Server.IdleTimeoutSecs,
		GPUProvider:         h.admin.GPUConfig().Provider,
		GPUBinary:           h.admin.GPUConfig().Binary,
		GPUIndex:            h.admin.GPUConfig().Index,
		DefaultArgs:         sys.Local.DefaultArgs,
		ServiceClientID:     id,
		ServiceClientSecret: secret,
		PrimaryModel:        reg.PrimaryModel,
		FallbackModel:       reg.FallbackModel,
		Providers:           h.getProvidersView(),
		Guardrails:          tools.GetDefaultGuardrails(h.admin.RootDir()),
		Communication:       reg.Communication,
		Search:              reg.Search,
	}
	respondJSON(w, cfg)
}

// AdminSystemHandler handles GET /admin/api/system
func (h *AdminHandlers) AdminSystemHandler(w http.ResponseWriter, r *http.Request) {
	sys := h.admin.GetSystem()
	view := adminSystemView{
		Bind:            sys.Server.Bind,
		ModelHost:       sys.Server.ModelHost,
		IdleTimeoutSecs: sys.Server.IdleTimeoutSecs,
		WorkspacesDir:   sys.WorkspacesDir,
		GPU:             h.admin.GPUConfig(),
		Environment:     sys.Server.Environment,
	}
	view.Local.ModelDir = sys.Local.ModelDir
	view.Local.LlamaServerBinary = sys.Local.LlamaServerBinary
	view.Local.DefaultArgs = sys.Local.DefaultArgs

	respondJSON(w, view)
}

// AdminSystemPutHandler handles PUT /admin/api/system
func (h *AdminHandlers) AdminSystemPutHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SystemUpdatePayload
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.admin.UpdateSettings(r.Context(), req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update system: "+err.Error())
		return
	}

	respondJSON(w, h.admin.GetSystem())
}

func (h *AdminHandlers) AdminConfigUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.SystemUpdatePayload
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err2 := h.admin.UpdateSettings(r.Context(), req); err2 != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update system: "+err2.Error())
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
