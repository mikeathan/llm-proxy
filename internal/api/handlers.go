package api

import (
	"encoding/json"
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/ratelimiter"
	"net/http"
)

type AssistantMessageHandler struct {
	provider device_context.DeviceContextProvider
	limiter  *ratelimiter.Limiter
	logger   logging.Logger
}

func NewAssistantMessageHandler(provider device_context.DeviceContextProvider, limiter *ratelimiter.Limiter, logger logging.Logger) *AssistantMessageHandler {
	return &AssistantMessageHandler{provider: provider, limiter: limiter, logger: logger}
}

func (h *AssistantMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	payload := &AssistantMessage{}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		h.logger.Error("failed to decode request body", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx, err := h.provider.GetDeviceContext()
	if err != nil {
		h.logger.Error("failed to get device context", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get device context")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ctx)
}
