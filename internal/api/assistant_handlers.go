package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/assistant" // Kept for DefaultSummaryMaxLen if needed, or remove?
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
	provider nodeherder.MCPService
	client   proxy.LLMClientProvider
	limiter  ratelimiter.Limiter
	logger   logging.Logger
	engine   assistant.Engine
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

	return h.runAgentLoop(ctx, client, history, log)
}

// runAgentLoop drives the agent execution model→tool→model iteration.
func (h *AssistantMessageHandler) runAgentLoop(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger) (any, *handlerError) {
	const maxSteps = 10
	const maxModelRetries = 3

	for step := 0; step < maxSteps; step++ {
		var msg proxy.Message
		var err *handlerError

		for attempt := 0; attempt < maxModelRetries; attempt++ {
			msg, err = h.callModel(ctx, client, history, log)
			if err == nil {
				break
			}
			log.Warn("llm retry", "attempt", attempt+1, "error", err.Message)
		}
		if err != nil {
			return nil, err
		}

		// Append Assistant's response to history immediately
		history = append(history, msg)

		if len(msg.ToolCalls) == 0 {
			return map[string]any{"reply": msg.Content}, nil
		}

		if err := h.processToolCall(ctx, msg, &history, log); err != nil {
			log.Error("tool execution failed", "error", err.Message)
			return nil, err
		}
	}

	return nil, &handlerError{Status: 500, Message: "agent exceeded step limit"}
}

func (h *AssistantMessageHandler) callModel(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger) (proxy.Message, *handlerError) {
	// Dynamic Tool Discovery
	availableTools, toolErr := h.provider.ListTools(ctx)
	if toolErr != nil {
		log.Error("failed to list tools", "error", toolErr)
		return proxy.Message{}, &handlerError{Status: http.StatusInternalServerError, Message: "failed to discover tools"}
	}

	req := proxy.ChatRequest{
		Messages:   history,
		Tools:      availableTools,
		ToolChoice: proxy.ToolChoiceAuto,
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		log.Error("LLM chat failed", "error", err)
		return proxy.Message{}, &handlerError{Status: http.StatusBadGateway, Message: "LLM request failed"}
	}

	if len(resp.Choices) == 0 {
		return proxy.Message{}, &handlerError{Status: http.StatusBadGateway, Message: "empty response from model"}
	}

	msg := resp.Choices[0].Message
	log.Debug("llm step", "tool_calls", len(msg.ToolCalls), "content_len", len(msg.Content))

	// Some local models (e.g. qwen2.5-coder) embed tool calls as XML markup in
	// Content instead of using the standard tool_calls JSON field.
	// Detect and normalise this so the agent loop behaves identically.
	if len(msg.ToolCalls) == 0 && msg.Content != "" {
		if parsed, ok := proxy.ParseContentToolCalls(msg.Content); ok {
			log.Debug("detected content-embedded tool calls, normalising", "count", len(parsed))
			msg.ToolCalls = parsed
			msg.Content = ""
		}
	}

	return msg, nil
}

func (h *AssistantMessageHandler) processToolCall(
	ctx context.Context,
	msg proxy.Message,
	history *[]proxy.Message,
	log logging.Logger,
) *handlerError {

	for _, tc := range msg.ToolCalls {
		log.Debug("llm tool call", "name", tc.Function.Name, "args", truncate(tc.Function.Arguments, 500))

		result, err := h.engine.ExecuteTool(ctx, tc)
		if err != nil {
			// In standard MCP/LLM flows, a tool failure should ideally be fed back to the LLM
			// so it can retry or apologize. For now, we log and return error to break loop,
			// or we could append the error as tool result?
			// Let's append error as result to allow LLM to recover.
			log.Warn("tool execution error (feeding back to llm)", "name", tc.Function.Name, "error", err)
			h.appendToolResult(history, tc, map[string]string{"error": err.Error()})
			continue
		}

		h.appendToolResult(history, tc, result)
	}
	return nil
}

func (h *AssistantMessageHandler) appendToolResult(history *[]proxy.Message, toolCall proxy.ToolCall, result any) {
	// Serialize result to JSON
	jsonResult, err := json.Marshal(result)
	if err != nil {
		h.logger.Error("failed to marshal tool result", "error", err)
		jsonResult = []byte(`{"error": "failed to serialize tool result"}`)
	}

	*history = append(*history, proxy.Message{
		Role:       proxy.ToolRole,
		Content:    string(jsonResult),
		ToolCallID: toolCall.ID,
	})
}

// Logic fix for appendToolResult structure:
// Refactoring processToolCall to first append the assistant message.

func (h *AssistantMessageHandler) getLLMClient(ctx context.Context, log logging.Logger) (proxy.Client, *handlerError) {
	client, err := h.client.GetClient(ctx)
	if err != nil {
		if errors.Is(err, llm.ErrModelStarting) {
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
