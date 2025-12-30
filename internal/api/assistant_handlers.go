package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
)

type AssistantMessage struct {
	ConversationID string `json:"conversation_id"`
	ContextVersion string `json:"context_version,omitempty"`
	Message        string `json:"message"`
	Timezone       string `json:"timezone,omitempty"`
}

type AssistantMessageHandler struct {
	provider  nodeherder.NodeHerderService
	client    proxy.LLMClientProvider
	limiter   ratelimiter.Limiter
	logger    logging.Logger
	assistant AssistantService
}

func NewAssistantMessageHandler(service AssistantService) *AssistantMessageHandler {

	return &AssistantMessageHandler{
		provider:  service.NodeHerder(),
		client:    service.ClientProvider(),
		limiter:   service.Limiter(),
		logger:    service.Logger(),
		assistant: service,
	}
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

	client, err := h.client.GetClient(r.Context())
	if err != nil {
		if errors.Is(err, llm.ErrModelStarting) {
			writeJSONError(w, http.StatusServiceUnavailable, "model is starting try again in a few seconds")
			return
		}

		h.logger.Error("failed to get LLM client", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get LLM client")
		return
	}

	req := buildChatRequest(payload, ctx)
	resp, err := client.Chat(r.Context(), req)

	if err != nil {
		h.logger.Error("LLM request failed", "error", err)
		writeJSONError(w, http.StatusBadGateway, "LLM request failed")
		return
	}

	if len(resp.Choices) == 0 {
		writeJSONError(w, http.StatusBadGateway, "empty response from model")
		return
	}

	// handle tool call from LLM
	choice := resp.Choices[0]
	if len(choice.ToolCalls) > 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

		// TODO - handle tool calling
		return
	}

	// handle LLM reply
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"reply": choice.Message.Content,
	})

}

func buildChatRequest(payload *AssistantMessage, ctx *nodeherder.LLMDeviceContext) proxy.ChatRequest {
	systemMsg := fmt.Sprintf(
		"Conversation ID: %s\nContext Version: %s\nTimezone: %s\n\nDevice Context:\n%s",
		payload.ConversationID,
		payload.ContextVersion,
		payload.Timezone,
		ctx.String(),
	)

	return proxy.ChatRequest{
		Messages: []proxy.Message{
			{
				Role:    proxy.SystemRole,
				Content: systemMsg,
			},
			{
				Role:    proxy.UserRole,
				Content: payload.Message,
			},
		},
	}
}
