package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/assistant"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/utils"
)

const (
	MaxRetries = 5
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

// handleAssistant executes a single agent cycle:
// 1. Build initial model context
// 2. Let the model reason and optionally request tools
// 3. Execute requested tools and feed results back to the model
// 4. Return the model's final answer
func (h *AssistantMessageHandler) handleAssistant(ctx context.Context, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {

	deviceCtx, err := h.loadDeviceContext(log)
	if err != nil {
		return nil, err
	}

	client, err := h.getLLMClient(ctx, log)
	if err != nil {
		return nil, err
	}

	history := h.buildInitialHistory(payload, deviceCtx)

	// runAgentLoop drives the model/tool execution cycle until the model
	// produces a final answer or the step limit is reached.
	return h.runAgentLoop(ctx, client, history, log)
}

func (h *AssistantMessageHandler) runAgentLoop(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger) (any, *handlerError) {

	for range MaxRetries {

		msg, err := h.callModel(ctx, client, history, log)
		if err != nil {
			return nil, err
		}

		if len(msg.ToolCalls) == 0 {
			return map[string]any{"reply": msg.Content}, nil
		}

		if err := h.processToolCall(ctx, msg, &history, log); err != nil {
			return nil, err
		}
	}

	return nil, &handlerError{Status: 500, Message: "agent exceeded step limit"}
}

func (h *AssistantMessageHandler) callModel(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger) (proxy.Message, *handlerError) {

	req := proxy.ChatRequest{
		Messages:   history,
		Tools:      []proxy.Tool{assistant.MetricsToolSchema()},
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

	return msg, nil
}

func (h *AssistantMessageHandler) processToolCall(ctx context.Context, msg proxy.Message, history *[]proxy.Message, log logging.Logger) *handlerError {

	tc := msg.ToolCalls[0]

	log.Debug("llm tool call", "name", tc.Function.Name, "args", truncate(tc.Function.Arguments, 500))

	result, err := h.engine.ExecuteTool(ctx, tc)
	if err != nil {
		return &handlerError{Status: 500, Message: "tool execution failed"}
	}

	// Record the model's tool request in the conversation history.
	// The assistant message contains the tool call but no user-facing content.
	// This is required so the model can continue reasoning with the tool result.
	*history = append(*history,
		proxy.Message{Role: proxy.AssistantRole, Content: "", ToolCalls: msg.ToolCalls},

		// Feed the tool's raw result back into the model as an observation.
		// This becomes the factual input for the model's next reasoning step.
		proxy.Message{Role: proxy.ToolRole, Content: utils.ToJson(result)},
	)

	return nil
}

func (h *AssistantMessageHandler) buildInitialHistory(payload *AssistantMessage, deviceCtx *nodeherder.LLMDeviceContext) []proxy.Message {

	return []proxy.Message{
		{
			Role: proxy.SystemRole,
			Content: assistant.BuildSystemMessage(
				payload.ConversationID,
				payload.ContextVersion,
				payload.Timezone,
				deviceCtx.String(),
			),
		},
		{
			Role:    proxy.UserRole,
			Content: payload.Message,
		},
	}
}

func (h *AssistantMessageHandler) loadDeviceContext(log logging.Logger) (*nodeherder.LLMDeviceContext, *handlerError) {
	deviceCtx, err := h.provider.GetDeviceContext()
	if err != nil {
		log.Error("get device context failed", "error", err)
		return nil, &handlerError{Status: 500, Message: "failed to get device context"}
	}
	return deviceCtx, nil
}

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
