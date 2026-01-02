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

type metricsArgs struct {
	DeviceID   string `json:"device_id"`
	Metric     string `json:"expose"`
	From       int64  `json:"from"`
	To         int64  `json:"to"`
	Aggregate  string `json:"aggregation,omitempty"`
	Resolution string `json:"resolution,omitempty"`
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

	h.logger.Info(
		"assistant request",
		"conversation", payload.ConversationID,
		"context_version", payload.ContextVersion,
		"timezone", payload.Timezone,
		"message_len", len(payload.Message),
	)
	ctx, err := h.provider.GetDeviceContext()
	if err != nil {
		h.logger.Error("failed to get device context", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get device context")
		return
	}

	client, err := h.client.GetClient(r.Context())
	if err != nil {
		if errors.Is(err, llm.ErrModelStarting) {
			h.logger.Error("LLM request failed - model starting")
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
		h.logger.Error("LLM request failed - empty response from model")
		writeJSONError(w, http.StatusBadGateway, "empty response from model")
		return
	}

	// handle tool call from LLM
	choice := resp.Choices[0]
	if len(choice.ToolCalls) > 0 {
		h.handleToolCall(w, r, choice.ToolCalls[0])
		return
	}

	// handle LLM reply
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"reply": choice.Message.Content,
	})
}

func (h *AssistantMessageHandler) handleToolCall(w http.ResponseWriter, r *http.Request, tc proxy.ToolCall) {
	if tc.Function.Name != "query_metrics" {
		h.logger.Error("Handle tool call failed - Unknown", "tool_name", tc.Function.Name)

		writeJSONError(w, http.StatusBadRequest, "unknown tool")
		return
	}

	var args metricsArgs
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		h.logger.Error("failed to parse tool arguments", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid tool arguments")
		return
	}

	queryReq := &nodeherder.MetricsQueryRequest{
		DeviceID:   args.DeviceID,
		Metric:     args.Metric,
		From:       args.From,
		To:         args.To,
		Aggregate:  args.Aggregate,
		Resolution: args.Resolution,
	}

	result, err := h.assistant.NodeHerder().QueryMetrics(r.Context(), queryReq)
	if err != nil {
		h.logger.Error("query metrics failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "query failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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
