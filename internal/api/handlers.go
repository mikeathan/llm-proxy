package api

import (
	"encoding/json"
	"llm-proxy/internal/context"
	"llm-proxy/internal/ratelimiter"
	"net/http"
)

// need limiter
// need cache
type AssistantMessageHandler struct {
	cache   *context.DeviceContextCache
	limiter *ratelimiter.Limiter
}

func NewAssistantMessageHandler(cache *context.DeviceContextCache, limiter *ratelimiter.Limiter) *AssistantMessageHandler {
	return &AssistantMessageHandler{cache: cache, limiter: limiter}
}

func (h *AssistantMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	payload := &context.AssistantMessage{}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx, ok := h.cache.Get()
	if !ok {
		// TODO
	}
}
