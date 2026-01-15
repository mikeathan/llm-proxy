package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"llm-proxy/internal/assistant"
	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/assistant/pending"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/utils"
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
	pending  pending.PendingToolCallStore
}

func NewAssistantMessageHandler(service AssistantService) *AssistantMessageHandler {

	return &AssistantMessageHandler{
		provider: service.NodeHerder(),
		client:   service.ClientProvider(),
		limiter:  service.Limiter(),
		logger:   service.Logger(),
		engine:   service.Engine(),
		pending:  service.Pending(),
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
	lockedIntent := ""

	// handleAssistant orchestrates a full agent cycle and shortcuts into the
	// clarification flow when a prior device match was ambiguous.
	if payload.ConversationID != "" {
		if result, ok := h.handlePending(ctx, payload, log); ok {
			return result, nil
		}
	}

	deviceCtx, err := h.loadDeviceContext(log)
	if err != nil {
		return nil, err
	}
	exposeIndex := tools.BuildExposeIndex(deviceCtx)

	client, err := h.getLLMClient(ctx, log)
	if err != nil {
		return nil, err
	}

	history := h.buildInitialHistory(payload, deviceCtx)

	// runAgentLoop drives the model/tool execution cycle until the model
	// produces a final answer or the step limit is reached.
	return h.runAgentLoop(ctx, client, history, log, payload.ConversationID, lockedIntent, exposeIndex)
}

// runAgentLoop drives the agent execution:
// The model is called repeatedly until it stops requesting tools.
// Each tool call is executed, its result is injected into the history,
// and the model is called again. When no tool calls remain, the model's
// message is considered the final answer and the loop exits.
func (h *AssistantMessageHandler) runAgentLoop(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger, conversationID string, lockedIntent string, exposeIndex map[tools.ExposeKey]nodeherder.LLMExpose) (any, *handlerError) {
	// runAgentLoop drives model→tool→model iteration, with separate retry budgets
	// for transient model failures vs tool execution failures.
	const maxSteps = 5
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

		if len(msg.ToolCalls) == 0 {
			return map[string]any{"reply": msg.Content}, nil
		}

		result, err := h.processToolCall(ctx, msg, &history, log, conversationID, &lockedIntent, exposeIndex)
		if err != nil {
			log.Error("tool execution failed", "error", err.Message)
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}

	return nil, &handlerError{Status: 500, Message: "agent exceeded step limit"}
}

func (h *AssistantMessageHandler) callModel(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger) (proxy.Message, *handlerError) {

	// callModel is the only place that invokes the LLM. Its errors are retried
	// separately from tool execution to preserve deterministic tool handling.
	req := proxy.ChatRequest{
		Messages:   history,
		Tools:      []proxy.Tool{tools.IntentToolSchema()},
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

func (h *AssistantMessageHandler) processToolCall(
	ctx context.Context,
	msg proxy.Message,
	history *[]proxy.Message,
	log logging.Logger,
	conversationID string,
	lockedIntent *string,
	exposeIndex map[tools.ExposeKey]nodeherder.LLMExpose,
) (any, *handlerError) {
	// processToolCall executes the requested tool and appends a compact history
	// for the model's next reasoning step.
	for _, tc := range msg.ToolCalls {

		if tc.Function.Name != "declare_intent" {
			return nil, &handlerError{
				Status:  400,
				Message: "invalid tool call: only declare_intent is allowed",
			}
		}

		log.Debug("llm tool call", "name", tc.Function.Name, "args", truncate(tc.Function.Arguments, 500))
		// declare_intent separates intent extraction (LLM) from deterministic execution.
		intent, err := tools.ParseIntentArgs(tc.Function.Arguments)
		if err != nil {
			return nil, &handlerError{Status: http.StatusBadRequest, Message: "invalid intent arguments"}
		}

		if *lockedIntent == "" {
			*lockedIntent = intent.Intent
		} else if intent.Intent != *lockedIntent {
			log.Warn("blocking intent drift", "from", *lockedIntent, "to", intent.Intent)

			*history = append(*history, proxy.Message{
				Role: proxy.ToolRole,
				Content: utils.ToJson(map[string]any{
					"error":  "Intent drift detected. Retry with same intent.",
					"rule":   "intent_locked",
					"intent": *lockedIntent,
				}),
			})
			return nil, nil
		}

		if err := tools.ValidateIntent(intent, exposeIndex); err != nil {
			// Inject the validation error back into the model and retry
			*history = append(*history, proxy.Message{
				Role: proxy.ToolRole,
				Content: utils.ToJson(map[string]any{
					"error": err.Error(),
					"rule":  "intent_invalid",
				}),
			})

			return nil, nil
		}

		metricCalls, err := tools.IntentToMetricsArgs(intent, &utils.RealClock{})
		if err != nil {
			return nil, &handlerError{Status: http.StatusBadRequest, Message: err.Error()}
		}

		// Map the intent into concrete query_metrics calls so the backend owns aggregation/time.
		for idx, args := range metricCalls {
			queryCall := proxy.ToolCall{
				ID:   fmt.Sprintf("%s-%d", tc.ID, idx),
				Type: "function",
				Function: proxy.FunctionCall{
					Name:      "query_metrics",
					Arguments: utils.ToJson(args),
				},
			}

			toolResult, err := h.engine.ExecuteTool(ctx, queryCall)
			if err != nil {
				if amb, ok := err.(*devices.AmbiguousDeviceError); ok {
					h.pending.Set(conversationID, pending.PendingToolCallState{
						ToolCall:   queryCall,
						History:    append([]proxy.Message(nil), (*history)...),
						Candidates: amb.Candidates,
						Target:     amb.Target,
						Expose:     amb.Expose,
					})
					reply := pending.FormatPendingPrompt(amb.Target, amb.Expose, amb.Candidates)
					return map[string]any{"reply": reply}, nil
				}
				var dErr *nodeherder.DomainError
				if errors.As(err, &dErr) {
					return nil, &handlerError{Status: dErr.Status, Message: dErr.Msg}
				}
			}

			// DEBUGGING LOGGING - Remove later
			b, _ := json.MarshalIndent(toolResult.Response, "", "  ")
			log.Debug("nodeherder response", "data", string(b))

			h.appendToolResult(history, []proxy.ToolCall{queryCall}, toolResult)
		}
	}
	return nil, nil
}

// Commit the executed tool call and its result into the conversation history
// so the model can continue the reasoning loop with the new information.
// This is particularly important for multi-step tool use when LLM replys back with a selection for the user to confirm.
func (h *AssistantMessageHandler) appendToolResult(history *[]proxy.Message, toolCalls []proxy.ToolCall, toolResult *assistant.ToolResult) {

	normalized := assistant.NormalizeMetrics(
		toolResult.Response,
		toolResult.Aggregation,
	)

	*history = append(*history, proxy.Message{
		Role:      proxy.AssistantRole,
		Content:   "",
		ToolCalls: toolCalls,
	})

	var observation any = normalized
	if normalized.Value == nil {
		observation = map[string]any{
			"note": "no data available for this query",
		}
	}

	*history = append(*history, proxy.Message{
		Role:    proxy.ToolRole,
		Content: utils.ToJson(observation),
	})
}

func (h *AssistantMessageHandler) buildInitialHistory(payload *AssistantMessage, deviceCtx *nodeherder.LLMDeviceContext) []proxy.Message {
	// Build a compact system prompt that caps device context to avoid context overflow.
	return []proxy.Message{
		{
			Role: proxy.SystemRole,
			Content: assistant.BuildSystemMessage(
				payload.ConversationID,
				payload.ContextVersion,
				payload.Timezone,
				deviceCtx.SummaryWithLimit(tools.DefaultSummaryMaxLen),
			),
		},
		{
			Role:    proxy.UserRole,
			Content: payload.Message,
		},
	}
}

// handlePending resolves a user selected device for an ambiguous match
// without re-entering the LLM until the tool result is ready.
func (h *AssistantMessageHandler) handlePending(ctx context.Context, payload *AssistantMessage, log logging.Logger) (any, bool) {
	state, ok := h.pending.Get(payload.ConversationID)
	if !ok {
		return nil, false
	}

	candidate, resolved := pending.ResolvePendingToolCall(payload.Message, state.Candidates)
	if !resolved {
		reply := pending.FormatPendingPrompt(state.Target, state.Expose, state.Candidates)
		return map[string]any{"reply": reply}, true
	}

	h.pending.Clear(payload.ConversationID)

	toolResult, execErr := h.engine.ExecuteToolWithDevice(ctx, state.ToolCall, candidate.ID)
	if execErr != nil {
		log.Error("tool execution failed after clarification", "error", execErr)

		var dErr *nodeherder.DomainError
		if errors.As(execErr, &dErr) {
			return map[string]any{"reply": dErr.Msg}, true
		}
		return map[string]any{"reply": "Failed to run the metrics query after clarification."}, true
	}

	history := append([]proxy.Message(nil), state.History...)
	h.appendToolResult(&history, []proxy.ToolCall{state.ToolCall}, toolResult)

	client, herr := h.getLLMClient(ctx, log)
	if herr != nil {
		return map[string]any{"reply": "Metrics collected, but failed to reach the model."}, true
	}

	deviceCtx, derr := h.loadDeviceContext(log)
	if derr != nil {
		return map[string]any{"reply": "Failed to load device context for the pending selection."}, true
	}
	exposeIndex := tools.BuildExposeIndex(deviceCtx)

	lockedIntent := ""
	result, err := h.runAgentLoop(ctx, client, history, log, payload.ConversationID, lockedIntent, exposeIndex)
	if err != nil {
		msg, herr := h.callModel(ctx, client, history, log)
		if herr == nil && len(msg.ToolCalls) == 0 {
			return map[string]any{"reply": msg.Content}, true
		}
		return map[string]any{"reply": "Metrics collected, but failed to complete the response."}, true
	}

	return result, true
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
