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

	// Check if user is asking about multiple devices - we only support single-device queries
	if multiDevices := devices.DetectMultipleDevices(payload.Message, deviceCtx); len(multiDevices) > 0 {
		return map[string]any{
			"reply": "I can only query one device at a time. You mentioned multiple devices: " +
				formatDeviceList(multiDevices) + ". Please ask about each device separately.",
		}, nil
	}

	client, err := h.getLLMClient(ctx, log)
	if err != nil {
		return nil, err
	}

	history := h.buildInitialHistory(payload, deviceCtx)

	// runAgentLoop drives the model/tool execution cycle until the model
	// produces a final answer or the step limit is reached.
	return h.runAgentLoop(ctx, client, history, log, payload.ConversationID, exposeIndex, deviceCtx, payload.Timezone)
}

// runAgentLoop drives the agent execution:
// The model is called repeatedly until it stops requesting tools.
// Each tool call is executed, its result is injected into the history,
// and the model is called again. When no tool calls remain, the model's
// message is considered the final answer and the loop exits.
func (h *AssistantMessageHandler) runAgentLoop(ctx context.Context, client proxy.Client, history []proxy.Message, log logging.Logger, conversationID string, exposeIndex map[tools.ExposeKey]nodeherder.LLMExpose, deviceCtx *nodeherder.LLMDeviceContext, timezone string) (any, *handlerError) {
	// runAgentLoop drives model→tool→model iteration, with separate retry budgets
	// for transient model failures vs tool execution failures.
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

		if len(msg.ToolCalls) == 0 {
			return map[string]any{"reply": msg.Content}, nil
		}

		result, err := h.processToolCall(ctx, msg, &history, log, conversationID, exposeIndex, deviceCtx, timezone)
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
	exposeIndex map[tools.ExposeKey]nodeherder.LLMExpose,
	deviceCtx *nodeherder.LLMDeviceContext,
	timezone string,
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

		// Filter metrics to only those mentioned in the user message
		// This prevents the LLM from over-fetching metrics the user didn't ask for
		userMessage := extractUserMessage(*history)
		mentionedMetrics := tools.ExtractMentionedMetrics(userMessage, deviceCtx)
		if len(mentionedMetrics) > 0 && len(intent.Metrics) > 0 {
			intent.Metrics = tools.FilterMetricsByMentioned(intent.Metrics, mentionedMetrics)
		}

		// Resolve device name fuzzily before validation
		if intent.TargetName != "" && len(intent.Metrics) > 0 {
			for _, metric := range intent.Metrics {
				// Try to resolve the device to a canonical name
				resolved, err := devices.ResolveDevice(deviceCtx, intent.TargetName, metric)
				if err != nil {
					// If ambiguous, return clarification prompt immediately
					if amb, ok := err.(*devices.AmbiguousDeviceError); ok {
						h.pending.Set(conversationID, pending.PendingToolCallState{
							// Reconstruct the original tool call for the pending state so we can retry it later
							ToolCall:   tc,
							History:    append([]proxy.Message(nil), (*history)...),
							Candidates: amb.Candidates,
							Target:     amb.Target,
							Expose:     amb.Expose,
						})
						reply := pending.FormatPendingPrompt(amb.Target, amb.Expose, amb.Candidates)
						return map[string]any{"reply": reply}, nil
					}
					log.Debug("failed to resolve device fuzzily", "target", intent.TargetName, "error", err)
				} else {
					// Update to canonical name
					intent.TargetName = resolved.Name
					// Break after first successful resolution
					break
				}
			}
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

		metricCalls, err := tools.IntentToMetricsArgs(intent, &utils.RealClock{}, exposeIndex, timezone)
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
				// Any other error from execution should be treated as a failure
				log.Error("tool execution failed", "error", err)
				return nil, &handlerError{Status: http.StatusInternalServerError, Message: err.Error()}
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
		toolResult.LookbackExpanded,
		toolResult.DeviceName,
	)

	*history = append(*history, proxy.Message{
		Role:      proxy.AssistantRole,
		Content:   "",
		ToolCalls: toolCalls,
	})

	var observation any = normalized
	if normalized.Value == nil && normalized.Note == "" {
		observation = map[string]any{
			"note": "no data available for this query",
		}
	}

	content := utils.ToJson(observation)
	// Log the exact DATA sent to the LLM to verify timestamp fidelity
	h.logger.Debug("tool output to llm", "content", content)

	*history = append(*history, proxy.Message{
		Role:    proxy.ToolRole,
		Content: content,
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

	// Update the original intent with the resolved device name so the standard flow can process it.
	intent, err := tools.ParseIntentArgs(state.ToolCall.Function.Arguments)
	if err != nil {
		log.Error("failed to parse pending intent", "error", err)
		return map[string]any{"reply": "Failed to process selection due to internal error."}, true
	}

	// Update target name with resolved device
	intent.TargetName = candidate.Name

	updatedArgs := utils.ToJson(intent)
	state.ToolCall.Function.Arguments = updatedArgs

	// Create a synthetic message for processToolCall
	msg := proxy.Message{
		ToolCalls: []proxy.ToolCall{state.ToolCall},
	}

	// Reconstruct history: The pending state history includes up to the point of ambiguity.
	// We pass a pointer to a copy of it so processToolCall can append results.
	historyCtx := append([]proxy.Message(nil), state.History...)

	deviceCtx, loadErr := h.loadDeviceContext(log)
	if loadErr != nil {
		return map[string]any{"reply": "Failed to load device context."}, true
	}
	exposeIndex := tools.BuildExposeIndex(deviceCtx)

	// We need the timezone from the current payload if available, or fall back to something?
	// The pending state doesn't store timezone. We should use the current request's timezone
	// since the user is interacting now.
	timezone := payload.Timezone

	result, callErr := h.processToolCall(ctx, msg, &historyCtx, log, payload.ConversationID, exposeIndex, deviceCtx, timezone)

	if callErr != nil {
		log.Error("process tool call failed after clarification", "error", callErr.Message)
		// If processToolCall failed, return a generic error or the message
		return map[string]any{"reply": "Failed to execute request after selection."}, true
	}

	if result != nil {
		return result, true // Likely the final reply map
	}

	// If processToolCall returned nil, it means it successfully executed tools and appended to history.
	// We now need to execute the agent loop to generate the final response.
	// We use the updated historyCtx which now contains the tool results.

	client, herr := h.getLLMClient(ctx, log)
	if herr != nil {
		return map[string]any{"reply": "Metrics collected, but failed to reach the model."}, true
	}

	loopResult, lerr := h.runAgentLoop(ctx, client, historyCtx, log, payload.ConversationID, exposeIndex, deviceCtx, timezone)
	if lerr != nil {
		log.Error("agent loop failed after clarification", "error", lerr.Message)
		return map[string]any{"reply": "Failed to generate response after collecting metrics."}, true
	}

	return loopResult, true
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

// extractUserMessage finds the first user message in the history.
func extractUserMessage(history []proxy.Message) string {
	for _, msg := range history {
		if msg.Role == proxy.UserRole && msg.Content != "" {
			return msg.Content
		}
	}
	return ""
}

// formatDeviceList formats a list of device names for user-friendly display.
func formatDeviceList(devices []string) string {
	if len(devices) == 0 {
		return ""
	}
	if len(devices) == 1 {
		return devices[0]
	}
	if len(devices) == 2 {
		return devices[0] + " and " + devices[1]
	}
	// 3 or more: "A, B, and C"
	result := ""
	for i, d := range devices {
		if i == len(devices)-1 {
			result += "and " + d
		} else {
			result += d + ", "
		}
	}
	return result
}
