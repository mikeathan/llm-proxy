package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/core/assistant" // Kept for DefaultSummaryMaxLen if needed, or remove?
	"llm-proxy/models"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/ratelimiter"
)

type AssistantMessage struct {
	ConversationID string `json:"conversation_id"`
	ContextVersion string `json:"context_version,omitempty"`
	Message        string `json:"message"`
	Timezone       string `json:"timezone,omitempty"`
}

type AssistantMessageHandler struct {
	provider   assistant.ToolProvider
	client     proxy.LLMClientProvider
	limiter    ratelimiter.Limiter
	logger     logging.Logger
	engine     assistant.Engine
	guardrails *assistant.GuardrailEngine
}

func NewAssistantMessageHandler(service AssistantService) *AssistantMessageHandler {
	return &AssistantMessageHandler{
		provider:   service.ToolProvider(),
		client:     service.ClientProvider(),
		limiter:    service.Limiter(),
		logger:     service.Logger(),
		engine:     service.Engine(),
		guardrails: service.GuardrailEngine(),
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

// handleAssistant executes a single agent cycle.
func (h *AssistantMessageHandler) handleAssistant(ctx context.Context, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {
	client, err := h.getLLMClient(ctx, log)
	if err != nil {
		return nil, err
	}

	history, histErr := h.buildInitialHistory(payload)
	if histErr != nil {
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: "failed to build history"}
	}

	// Initialize the unified Agent
	agent := assistant.NewAgent(client, h.provider, h.engine, assistant.AgentOptions{
		Logger:     log,
		MaxSteps:   10,
		Guardrails: h.guardrails,
	})

	reply, _, agErr := agent.Execute(ctx, history)
	if agErr != nil {
		log.Error("agent execution failed", "error", agErr)
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: agErr.Error()}
	}

	return map[string]any{"reply": reply}, nil
}

// Logic fix for appendToolResult structure:
// Refactoring processToolCall to first append the assistant message.

func (h *AssistantMessageHandler) getLLMClient(ctx context.Context, log logging.Logger) (proxy.Client, *handlerError) {
	client, err := h.client.GetClient(ctx)
	if err != nil {
		if errors.Is(err, models.ErrModelStarting) {
			return nil, &handlerError{
				Status:  http.StatusServiceUnavailable,
				Message: "model is starting, try again shortly",
			}
		}
		log.Error("get LLM client failed", "error", err)
		return nil, &handlerError{
			Status:  http.StatusInternalServerError,
			Message: "failed to get LLM client",
		}
	}
	return client, nil
}

func (h *AssistantMessageHandler) buildInitialHistory(payload *AssistantMessage) ([]proxy.Message, error) {
	systemPrompt, err := h.provider.GetSystemPrompt()
	if err != nil {
		h.logger.Error("failed to get system prompt", "error", err)
		return nil, err
	}

	return []proxy.Message{
		{
			Role: proxy.SystemRole,
			Content: assistant.BuildSystemMessage(
				systemPrompt,
				payload.ConversationID,
				payload.ContextVersion,
				payload.Timezone,
			),
		},
		{
			Role:    proxy.UserRole,
			Content: payload.Message,
		},
	}, nil
}
