package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"llm-proxy/internal/assistant"
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
	provider nodeherder.NodeHerderService
	client   proxy.LLMClientProvider
	limiter  ratelimiter.Limiter
	logger   logging.Logger
	engine   assistant.Engine
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
		provider: service.NodeHerder(),
		client:   service.ClientProvider(),
		limiter:  service.Limiter(),
		logger:   service.Logger(),
		engine:   service.Engine(),
	}
}

func (h *AssistantMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, log, ok := h.prepareRequest(w, r)
	if !ok {
		return
	}

	result, err := h.handleAssistant(r.Context(), payload, log)
	if err != nil {
		writeJSONError(w, err.Status, err.Message)
		return
	}

	respondJSON(w, result)
}

func (h *AssistantMessageHandler) prepareRequest(w http.ResponseWriter, r *http.Request) (*AssistantMessage, logging.Logger, bool) {

	var payload AssistantMessage
	if err := decodeJSON(r, &payload); err != nil {
		if errors.Is(err, ErrUnsupportedContentType) {
			writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		} else {
			h.logger.Error("failed to decode request body", "error", err)
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return nil, nil, false
	}

	traceID := getTraceID(payload.ConversationID, r.RemoteAddr)
	log := h.logger.With("trace", traceID)

	if !h.limiter.Allow(traceID, time.Second) {
		log.Warn("assistant rate limit")
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return nil, nil, false
	}

	log.Info(
		"assistant request",
		"context_version", payload.ContextVersion,
		"timezone", payload.Timezone,
		"message_len", len(payload.Message),
	)

	return &payload, log, true
}

func (h *AssistantMessageHandler) handleAssistant(ctx context.Context, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {

	deviceCtx, err := h.provider.GetDeviceContext()
	if err != nil {
		log.Error("get device context failed", "error", err)
		return nil, &handlerError{
			Status:  http.StatusInternalServerError,
			Message: "failed to get device context",
			Err:     err,
		}
	}

	client, err := h.client.GetClient(ctx)
	if err != nil {
		if errors.Is(err, llm.ErrModelStarting) {
			log.Warn("model starting")
			return nil, &handlerError{
				Status:  http.StatusServiceUnavailable,
				Message: "model is starting, try again shortly",
				Err:     err,
			}
		}

		log.Error("get LLM client failed", "error", err)
		return nil, &handlerError{
			Status:  http.StatusInternalServerError,
			Message: "failed to get LLM client",
			Err:     err,
		}
	}

	req := buildChatRequest(payload, deviceCtx)

	resp, err := client.Chat(ctx, req)
	if err != nil {
		log.Error("LLM chat failed", "error", err)
		return nil, &handlerError{
			Status:  http.StatusBadGateway,
			Message: "LLM request failed",
			Err:     err,
		}
	}

	if len(resp.Choices) == 0 {
		log.Error("empty LLM response")
		return nil, &handlerError{
			Status:  http.StatusBadGateway,
			Message: "empty response from model",
		}
	}

	return h.handleChoice(ctx, resp.Choices[0], log)
}

func (h *AssistantMessageHandler) handleChoice(ctx context.Context, choice proxy.Choice, log logging.Logger) (any, *handlerError) {

	log.Debug(
		"llm response",
		"tool_calls", len(choice.ToolCalls),
		"content_len", len(choice.Message.Content),
	)

	if len(choice.ToolCalls) > 0 {
		tc := choice.ToolCalls[0]

		log.Debug(
			"llm tool call",
			"name", tc.Function.Name,
			"args", truncate(tc.Function.Arguments, 500),
		)

		result, err := h.engine.ExecuteTool(ctx, tc)
		if err != nil {
			log.Error("tool execution failed", "error", err)
			return nil, &handlerError{
				Status:  http.StatusInternalServerError,
				Message: "query failed",
				Err:     err,
			}
		}

		return result, nil
	}

	return map[string]any{
		"reply": choice.Message.Content,
	}, nil
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
