package api

import (
	"net/http"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

func (h *AdminHandlers) AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	sys := h.admin.GetSystem()
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
		PrimaryModel:        sys.Server.PrimaryModel,
		FallbackModel:       sys.Server.FallbackModel,
		Providers:           h.getProvidersView(),
		Guardrails:          tools.GetDefaultGuardrails(h.admin.RootDir()),
		Communication:       sys.Communication,
		Search:              sys.Search,
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

	// 3. Sync All Providers into Registry (handled by UpdateSettings, but here we might need to handle the loop separately if registry was complex)
	// Actually, UpdateSettings handles it now.

	w.WriteHeader(http.StatusNoContent)
}
