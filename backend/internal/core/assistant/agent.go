package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxSteps is the fallback when no per-model value is set.
	DefaultMaxSteps = 25
	// DefaultContextBudget is the character count that triggers the physical sieve.
	DefaultContextBudget = 15000
	// AgentGlobalTimeout is the maximum duration for a complete agentic operation.
	AgentGlobalTimeout = 30 * time.Minute
	// AgentTurnTimeout is the maximum time allowed for a single LLM turn.
	AgentTurnTimeout = 10 * time.Minute
	// AgentRetryTimeout is the timeout used for fallback/retry logic.
	AgentRetryTimeout = 5 * time.Minute
)

// Agent represents a unified, stateful assistant that can use tools.
type Agent struct {
	client         proxy.Client
	provider       ToolProvider
	engine         Engine
	guardrails     *guardrails.GuardrailEngine
	logger         logging.Logger
	maxSteps       int
	contextBudget  int
	useNativeTools bool
	observer       Observer
	workspaceID    string
	onGuardrail    GuardrailDecisionCallback
}

type AgentOptions struct {
	MaxSteps                 int
	ContextBudget            int // 0 = use DefaultContextBudget
	Logger                   logging.Logger
	Guardrails               *guardrails.GuardrailEngine
	Observer                 Observer
	WorkspaceID              string
	UseNativeTools           *bool // nil = delegate to provider; explicit true/false overrides
	UsePrefill               bool  // ignored; prefill is always on for automation + text tools
	GuardrailDecisionHandler GuardrailDecisionCallback
}

// NewAgent creates a new unified agent.
func NewAgent(client proxy.Client, provider ToolProvider, engine Engine, opts AgentOptions) *Agent {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = DefaultMaxSteps
	}
	if opts.ContextBudget <= 0 {
		opts.ContextBudget = DefaultContextBudget
	}
	if opts.Logger == nil {
		opts.Logger = logging.NewNopLogger()
	}
	// Default guardrail engine if none provided
	gr := opts.Guardrails
	if gr == nil {
		gr = guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil)
	}

	// Resolve UseNativeTools: explicit override takes precedence, otherwise ask provider
	useNative := provider.UseNativeTools()
	if opts.UseNativeTools != nil {
		useNative = *opts.UseNativeTools
	}

	return &Agent{
		client:         client,
		provider:       provider,
		engine:         engine,
		guardrails:     gr,
		logger:         opts.Logger,
		maxSteps:       opts.MaxSteps,
		contextBudget:  opts.ContextBudget,
		useNativeTools: useNative,
		observer:       opts.Observer,
		workspaceID:    opts.WorkspaceID,
		onGuardrail:    opts.GuardrailDecisionHandler,
	}
}

// Execute runs the agentic loop for a given conversation history.
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	execCtx, cancel := context.WithTimeout(ctx, AgentGlobalTimeout)
	defer cancel()

	steps := 0
	currentHistory := append([]proxy.Message{}, history...)
	type toolKey struct{ name, args string }
	recentCalls := make([]toolKey, 0, 3)
	duplicateStreak := 0
	const maxDuplicateStreak = 3
	parseErrorStreak := 0
	var lastParseErrorKind string // "no_xml", "json", or "tool_name"

	for steps < a.maxSteps {
		steps++
		if err := execCtx.Err(); err != nil {
			return "", currentHistory, fmt.Errorf("agent execution halted: %w", err)
		}

		a.notifyStepStart(steps)
		a.notifyThinking()

		var turnMsg proxy.Message
		var parseErr *proxy.ParseError
		var toolsList []proxy.Tool
		err := func() error {
			turnCtx, turnCancel := context.WithTimeout(execCtx, AgentTurnTimeout)
			defer turnCancel()

			var err error
			toolsList, err = a.provider.ListTools(turnCtx)
			if err != nil {
				return fmt.Errorf("failed to list tools: %w", err)
			}

			// PHYSICAL SIEVE: Prune middle history when nearing context limits
			totalChars := 0
			for _, m := range currentHistory {
				totalChars += len(m.Content)
			}
			if totalChars > a.contextBudget {
				a.logger.Warn("critical context pressure - activating physical sieve", "chars", totalChars)
				if len(currentHistory) > 10 {
					newHistory := make([]proxy.Message, 0, 10)
					newHistory = append(newHistory, currentHistory[0], currentHistory[1])
					newHistory = append(newHistory, proxy.Message{
						Role:    proxy.SystemRole,
						Content: prompts.SieveSystemNote,
					})
					newHistory = append(newHistory, currentHistory[len(currentHistory)-10:]...)
					currentHistory = newHistory

					// Do NOT reset recentCalls — repetition detection must survive
					// the sieve boundary to prevent loops spanning across it.

					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: prompts.ContextSieveWarning,
					})
				}
			}

			msg, err := a.computeNextResponse(turnCtx, currentHistory, toolsList)
			if err != nil {
				return err
			}

			// DIAGNOSTIC: Log raw model response at INFO level so we can see
			// exactly what the model produces — critical for debugging local models.
			a.logger.Info("raw model response",
				"content_len", len(msg.Content),
				"content", msg.Content,
				"native_tool_calls", len(msg.ToolCalls),
			)

			// Extract tools and CLEAN the message content to save tokens
			parseErr = a.handleContentToolCalls(&msg)
			turnMsg = msg

			if parseErr != nil {
				a.logger.Warn("tool call parse error",
					"xml_found", parseErr.XMLFound,
					"json_error", parseErr.JSONError,
					"tool_name", parseErr.ToolName,
				)
			}

			if len(turnMsg.ToolCalls) > 0 {
				// Validate tool names against available tools.
				// Reset parseErr first — the XML parse error from
				// handleContentToolCalls is stale when native tool
				// calls were produced (e.g. after switching to native
				// tools due to thinking-mode prefill rejection).
				parseErr = nil
				for _, tc := range turnMsg.ToolCalls {
					if valErr := proxy.ValidateToolCall(tc, toolsList); valErr != nil {
						if pe, ok := valErr.(*proxy.ParseError); ok {
							parseErr = pe
						}
						break
					}
				}
				if parseErr != nil {
					parseErr.XMLFound = true
					turnMsg.ToolCalls = nil // clear invalid calls
					return nil
				}

				// Deduplicate and Repetition Detection
				uniqueCalls := make([]proxy.ToolCall, 0, len(turnMsg.ToolCalls))
				seenInTurn := make(map[string]bool)
				for _, tc := range turnMsg.ToolCalls {
					callKey := tc.Function.Name + ":" + tc.Function.Arguments
					if !seenInTurn[callKey] {
						seenInTurn[callKey] = true
						uniqueCalls = append(uniqueCalls, tc)
					}
				}
				turnMsg.ToolCalls = uniqueCalls

				// Tool calls parsed and validated successfully — reset the error streak.
				parseErrorStreak = 0
				lastParseErrorKind = ""

				for _, tc := range turnMsg.ToolCalls {
					key := toolKey{tc.Function.Name, tc.Function.Arguments}
					if tc.Function.Name != models.ToolSubmitFinalAnswer && tc.Function.Name != models.ToolSystemError {
						if len(recentCalls) > 0 && recentCalls[len(recentCalls)-1] == key {
							a.logger.Warn("duplicate action detected", "tool", key.name, "args", key.args, "streak", duplicateStreak+1)
							duplicateStreak++
							if duplicateStreak >= maxDuplicateStreak {
								return fmt.Errorf("infinite loop detected: model keeps calling %s(%s) after %d nags", key.name, key.args, duplicateStreak)
							}
							currentHistory = append(currentHistory, turnMsg)
							a.notify(EventMessage, turnMsg)
							currentHistory = append(currentHistory, proxy.Message{
								Role:    proxy.UserRole,
								Content: prompts.AutomationDuplicateNagPrompt,
							})
							return nil
						}
						if len(recentCalls) >= 3 {
							recentCalls = recentCalls[1:]
						}
						recentCalls = append(recentCalls, key)
					}
				}

				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
				if err := a.processToolCalls(turnCtx, turnMsg, &currentHistory); err != nil {
					return err
				}

				for _, tc := range turnMsg.ToolCalls {
					if tc.Function.Name == models.ToolSubmitFinalAnswer {
						turnMsg.Content = extractTaskSummary(tc.Function.Arguments)
						return fmt.Errorf("TASK_SUBMITTED")
					}
				}
				return nil
			}
			return nil
		}()

		if err != nil {
			if err.Error() == "TASK_SUBMITTED" {
				return turnMsg.Content, currentHistory, nil
			}
			return "", currentHistory, err
		}

		if len(turnMsg.ToolCalls) == 0 {
			if len(currentHistory) == 0 || currentHistory[len(currentHistory)-1].Content != turnMsg.Content {
				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
			}

			isAutomation := false
			for _, m := range history {
				if prompts.IsAutomationTask(m.Content) {
					isAutomation = true
					break
				}
			}

			if isAutomation {
				if a.isPrematureTermination(turnMsg, currentHistory) {
					a.logger.Warn("automation task — premature termination detected", "step", steps)
					return turnMsg.Content, currentHistory, nil
				}
				// If the model tried to call a tool but got the format wrong,
				// give specific feedback instead of a generic nag.
				if parseErr != nil {
					errKind := parseErrorKind(parseErr)
					if errKind == lastParseErrorKind {
						parseErrorStreak++
					} else {
						parseErrorStreak = 0
						lastParseErrorKind = errKind
					}

					availableNames := proxy.AvailableToolNames(toolsList)
					feedback := parseErr.Feedback(availableNames)

					// Escalate when the model keeps making the same mistake.
					// After 3 consecutive identical errors the feedback gets
					// more forceful and includes a concrete one-shot example.
					if parseErrorStreak >= 2 {
						feedback = fmt.Sprintf(prompts.ParseErrorEscalationPrefix, feedback)
					}

					a.logger.Info("injecting specific parse-error feedback",
						"error", parseErr.Error(),
						"streak", parseErrorStreak,
						"feedback", feedback,
					)
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: feedback,
					})
				} else {
					a.logger.Warn("turn resulted in no action - nagging model", "step", steps, "nag", prompts.AutomationNagPrompt)
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: prompts.AutomationNagPrompt,
					})
				}
				continue
			}

			if !isAutomation {
				if a.isPrematureTermination(turnMsg, currentHistory) {
					a.logger.Info("premature termination detected — model is repeating or producing empty output")
					return turnMsg.Content, currentHistory, nil
				}
				if steps == 1 {
					return turnMsg.Content, currentHistory, nil
				}
				if a.countConsecutiveChat(currentHistory) >= 2 {
					return turnMsg.Content, currentHistory, nil
				}
				if a.precededByToolResult(currentHistory) {
					return turnMsg.Content, currentHistory, nil
				}
			}
		}
	}
	return "", currentHistory, fmt.Errorf("agent exceeded max steps (%d)", a.maxSteps)
}

// parseErrorKind classifies a ParseError into a stable category so the
// escalation logic can detect when the model keeps making the same mistake.
func parseErrorKind(e *proxy.ParseError) string {
	if e == nil {
		return ""
	}
	if !e.XMLFound {
		return "no_xml"
	}
	if e.JSONError != "" {
		return "json"
	}
	if e.ToolName != "" {
		return "tool_name"
	}
	return ""
}

func (a *Agent) handleContentToolCalls(msg *proxy.Message) *proxy.ParseError {
	if msg.Content == "" {
		return nil
	}
	cleaned, calls, parseErr := proxy.ParseContentToolCalls(msg.Content)
	msg.Content = cleaned // strip XML tags to save context tokens
	if parseErr == nil && len(calls) > 0 {
		msg.ToolCalls = append(msg.ToolCalls, calls...)
		a.logger.Info("xml tool calls parsed from content", "count", len(calls))
		return nil
	}
	return parseErr
}

// precededByToolResult checks whether the last assistant message (no tool calls)
// follows a tool result, indicating that the model has just completed a tool loop.
func (a *Agent) precededByToolResult(history []proxy.Message) bool {
	if len(history) < 2 {
		return false
	}
	last := history[len(history)-1]
	prev := history[len(history)-2]
	return last.Role == proxy.AssistantRole &&
		len(last.ToolCalls) == 0 &&
		strings.TrimSpace(last.Content) != "" &&
		prev.Role == proxy.ToolRole
}

func (a *Agent) countConsecutiveChat(history []proxy.Message) int {
	chatCount := 0
	var lastContent string
	for j := len(history) - 1; j >= 0; j-- {
		if history[j].Role == proxy.AssistantRole && len(history[j].ToolCalls) == 0 {
			// Check for near-identical monologue repetition
			current := strings.TrimSpace(history[j].Content)
			if lastContent != "" && len(current) > 10 && strings.Contains(lastContent, current[:len(current)/2]) {
				chatCount += 2 // Penalize similar monologues heavily
			} else {
				chatCount++
			}
			lastContent = current
		} else {
			break
		}
	}
	return chatCount
}

func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	llmTools := tools
	if !a.useNativeTools {
		llmTools = nil
	}

	prepared := a.prepareMessages(history)

	isAutomationCtx := false
	for _, m := range history {
		if prompts.IsAutomationTask(m.Content) {
			isAutomationCtx = true
			break
		}
	}
	if (llmTools == nil || isAutomationCtx) && len(tools) > 0 {
		prepared = a.injectToolInstructions(prepared, tools)
	}

	// In automation mode with text-based tools, prefill the assistant
	// response so the model never needs to decide between thinking and
	// acting.  It receives `<tool_call>\n{"tool":"` as the last assistant
	// message and must complete the JSON with a tool name and arguments.
	var prefill string
	if isAutomationCtx && !a.useNativeTools {
		prefill = prompts.AutomationPrefline
		prepared = append(prepared, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: prefill,
		})
	}

	req := proxy.ChatRequest{
		Messages: prepared,
		Tools:    llmTools,
	}

	ch, err := a.client.Stream(ctx, req)
	if err != nil {
		// llama.cpp with enable_thinking rejects prefill assistant messages.
		// Switch to native tools with tool_choice=required — this forces the
		// model to call a tool on every turn without needing XML prefill.
		if prefill != "" && isPrefillThinkingError(err) {
			a.logger.Info("prefill rejected by server (thinking mode active), switching to native tools")
			a.useNativeTools = true
			prefill = ""
			llmTools = tools
			prepared = a.prepareMessages(history)
			req = proxy.ChatRequest{
				Messages:  prepared,
				Tools:     llmTools,
				ToolChoice: proxy.ToolChoiceRequired,
			}
			ch, err = a.client.Stream(ctx, req)
		}
		if err != nil {
			a.logger.Warn("streaming not supported or failed, falling back to non-streaming", "error", err)
			return a.computeNextResponseNonStreaming(ctx, history, tools)
		}
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := a.processStream(ctx, ch, &fullMsg); err != nil {
		return proxy.Message{}, err
	}

	if prefill != "" {
		fullMsg.Content = prefill + fullMsg.Content
	}

	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 && llmTools != nil {
		a.logger.Info("empty response with native tools, retrying without them")
		return a.computeNextResponseNonStreaming(ctx, history, nil)
	}

	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 {
		a.logger.Info("stream returned no content, falling back to non-streaming retry")
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	a.logger.Debug("raw model response received", "content_preview", fmt.Sprintf("%.50s", fullMsg.Content), "tool_calls", len(fullMsg.ToolCalls))
	return fullMsg, nil
}

func (a *Agent) processStream(ctx context.Context, ch <-chan *proxy.ChatResponse, fullMsg *proxy.Message) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-ch:
			if !ok {
				return nil
			}
			if len(resp.Choices) > 0 {
				choice := resp.Choices[0]
				chunkContent := choice.Delta.Content
				if chunkContent == "" && choice.Message.Content != "" {
					chunkContent = choice.Message.Content
				}
				reasoningChunk := choice.Delta.ReasoningContent
				if reasoningChunk == "" && choice.Message.ReasoningContent != "" {
					reasoningChunk = choice.Message.ReasoningContent
				}
				totalChunk := chunkContent + reasoningChunk
				if totalChunk != "" {
					fullMsg.Content += totalChunk
					displayContent, hasToolCall := FilterStreamingMarkup(fullMsg.Content)
					if hasToolCall {
						a.notify(EventToolStream, displayContent+"\n\n🛠️ *Agent is initiating tool calls...*")
					} else {
						a.notify(EventToolStream, displayContent)
					}
				}
				if len(choice.Delta.ToolCalls) > 0 {
					// Native tool-call delta accumulation.  This path only activates
					// when useNativeTools is true (cloud models using native tool
					// schemas).  When useNativeTools is false the LLM server never
					// sends tool-call deltas, and tool extraction happens via the
					// XML text parser in handleContentToolCalls instead.
					for _, tc := range choice.Delta.ToolCalls {
						if tc.ID != "" {
							fullMsg.ToolCalls = append(fullMsg.ToolCalls, tc)
						} else if len(fullMsg.ToolCalls) > 0 {
							last := &fullMsg.ToolCalls[len(fullMsg.ToolCalls)-1]
							last.Function.Arguments += tc.Function.Arguments
						}
					}
				}
			}
		}
	}
}

func (a *Agent) computeNextResponseNonStreaming(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()

	llmTools := tools
	if !a.useNativeTools {
		llmTools = nil
	}

	preparedHistory := a.prepareMessages(history)

	isAutomationCtx := false
	for _, m := range history {
		if prompts.IsAutomationTask(m.Content) {
			isAutomationCtx = true
			break
		}
	}
	if llmTools == nil && len(tools) > 0 {
		preparedHistory = a.injectToolInstructions(preparedHistory, tools)
	}

		// Prefill assistant response in automation mode so the model
	// never has to choose between thinking and acting.  This is
	// on by default for all text-based tool calling.
	var prefill string
	if isAutomationCtx && !a.useNativeTools {
		prefill = prompts.AutomationPrefline
		preparedHistory = append(preparedHistory, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: prefill,
		})
	}

	req := proxy.ChatRequest{
		Messages: preparedHistory,
		Tools:    llmTools,
	}

	if rawReq, err := json.Marshal(req); err == nil {
		a.logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	}

	resp, err := a.client.Chat(chatCtx, req)
	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.logger.Info("prefill rejected by server (thinking mode), switching to native tools (non-stream)")
		a.useNativeTools = true
		prefill = ""
		llmTools = tools
		req = proxy.ChatRequest{
			Messages:   a.prepareMessages(history),
			Tools:      llmTools,
			ToolChoice: proxy.ToolChoiceRequired,
		}
		resp, err = a.client.Chat(chatCtx, req)
	}
	if err != nil && isToolSupportError(err) {
		a.logger.Warn("model does not support tools, retrying without them", "error", err)
		a.notifyFallbackWarning(err)
		chatCtx2, cancel2 := context.WithTimeout(ctx, AgentRetryTimeout)
		defer cancel2()
		resp, err = a.client.Chat(chatCtx2, proxy.ChatRequest{Messages: history})
	}

	if err != nil {
		return proxy.Message{}, fmt.Errorf("llm completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return proxy.Message{}, fmt.Errorf("llm returned no choices")
	}

	msg := resp.Choices[0].Message
	if prefill != "" {
		msg.Content = prefill + msg.Content
	}
	return msg, nil
}

func (a *Agent) isPrematureTermination(msg proxy.Message, history []proxy.Message) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return len(msg.ToolCalls) == 0
	}
	if len(history) >= 2 {
		last := history[len(history)-1]
		if last.Role == proxy.AssistantRole && strings.TrimSpace(last.Content) == content {
			prev := history[len(history)-2]
			if prev.Role == proxy.AssistantRole && strings.TrimSpace(prev.Content) == content {
				return true
			}
		}
	}
	return false
}

func (a *Agent) processToolCalls(ctx context.Context, msg proxy.Message, history *[]proxy.Message) error {
	var mu sync.Mutex
	hasSubmit := false
	for _, tc := range msg.ToolCalls {
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			hasSubmit = true
			break
		}
	}
	if hasSubmit && len(msg.ToolCalls) > 1 {
		a.logger.Warn("rejected batched submission", "count", len(msg.ToolCalls))
		mu.Lock()
		errorMsg := prompts.AutomationRejectedSubmissionPrompt
		for _, tc := range msg.ToolCalls {
			a.appendToolResult(history, tc, map[string]string{"error": errorMsg})
		}
		mu.Unlock()
		return nil
	}

	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tc.Type != "" && tc.Type != "function" {
			continue
		}

		a.logger.Info("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)

		toolsList, _ := a.provider.ListTools(ctx)
		if err := validateToolArgs(tc, toolsList); err != nil {
			a.logger.Warn("tool argument validation failed", "name", tc.Function.Name, "error", err)
			mu.Lock()
			a.appendToolResult(history, tc, map[string]string{"error": fmt.Sprintf("INVALID ARGUMENTS: %v", err)})
			mu.Unlock()
			return nil
		}
		guardrailApproved := false

		if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
			a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
			a.notifyGuardrailViolation(tc.Function.Name, err)

			if a.onGuardrail != nil {
				decision, decErr := a.onGuardrail(ctx, GuardrailBlockedPayload{
					DecisionID: fmt.Sprintf("gr_%d", time.Now().UnixNano()),
					Tool:       tc.Function.Name,
					Args:       tc.Function.Arguments,
					Reason:     err.Error(),
					Category:   toolCategory(tc.Function.Name),
				})
				if decErr == nil && decision.Allow {
					guardrailApproved = true
					if decision.Persist {
						if err := a.guardrails.PersistOverride(a.workspaceID, toolCategory(tc.Function.Name), tc.Function.Name, tc.Function.Arguments); err != nil {
							a.logger.Warn("failed to persist guardrail override", "error", err)
						}
					}
				} else {
					mu.Lock()
					a.appendToolResult(history, tc, formatGuardrailError(err))
					mu.Unlock()
					return nil
				}
			} else {
				mu.Lock()
				a.appendToolResult(history, tc, formatGuardrailError(err))
				mu.Unlock()
				return nil
			}
		}

		a.notifyToolCall(tc)
		toolCtx := models.WithWorkspaceID(ctx, a.workspaceID)
		if guardrailApproved {
			toolCtx = models.WithGuardrailApproved(toolCtx)
		}
		result, err := a.engine.ExecuteTool(toolCtx, tc)
		mu.Lock()
		a.appendToolResult(history, tc, result)
			resultStr, _ := json.Marshal(result)
			a.logger.Info("tool execution completed", "name", tc.Function.Name, "error", err, "result", string(resultStr))
		a.notifyToolResult(tc.ID, tc.Function.Name, result)
		mu.Unlock()

		if err != nil {
			a.logger.Warn("tool execution failed - stopping batch", "name", tc.Function.Name, "error", err)
			return nil
		}
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			return nil
		}
	}
	return nil
}

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
}

func toolCategory(toolName string) string {
	switch toolName {
	case models.ToolTerminalExecute:
		return "terminal"
	case models.ToolDirectoryList, models.ToolFileRead, models.ToolFileWrite:
		return "filesystem"
	case models.ToolNetworkFetch, models.ToolNetworkScan, models.ToolNetworkInfo:
		return "network"
	case models.ToolInternetSearch:
		return "search"
	case models.ToolNotifyUser:
		return "communication"
	default:
		return "general"
	}
}

func extractTaskSummary(rawArgs string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "Task complete."
	}
	for _, key := range []string{"summary", "message", "report", "findings", "content", "result"} {
		if val, ok := args[key].(string); ok && val != "" {
			return val
		}
	}
	for _, val := range args {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return "Task complete."
}

func validateToolArgs(tc proxy.ToolCall, tools []proxy.Tool) error {
	var targetTool *proxy.Tool
	for _, t := range tools {
		if t.Function.Name == tc.Function.Name {
			targetTool = &t
			break
		}
	}
	if targetTool == nil {
		return fmt.Errorf("tool '%s' not found", tc.Function.Name)
	}
	params, ok := targetTool.Function.Parameters.(map[string]any)
	if !ok {
		return nil
	}
	requiredRaw, ok := params["required"]
	if !ok {
		return nil
	}
	var required []string
	switch r := requiredRaw.(type) {
	case []any:
		for _, v := range r {
			if s, ok := v.(string); ok {
				required = append(required, s)
			}
		}
	case []string:
		required = r
	}
	if len(required) == 0 {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse arguments as JSON: %w", err)
	}
	for _, field := range required {
		val, ok := args[field]
		if !ok {
			return fmt.Errorf("missing required parameter '%s'", field)
		}
		if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf("parameter '%s' cannot be empty", field)
		}
	}
	return nil
}

func (a *Agent) appendToolResult(history *[]proxy.Message, tc proxy.ToolCall, result any) {
	raw, _ := json.Marshal(result)
	strContent := string(raw)
	strContent = proxy.TruncateResult(strContent)
	*history = append(*history, proxy.Message{
		Role:       proxy.ToolRole,
		Content:    strContent,
		ToolCallID: tc.ID,
	})
}

func isToolSupportError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "tools is not currently supported") ||
		strings.Contains(lowErr, "tool_choice is not supported") ||
		strings.Contains(lowErr, "auto tool choice requires") ||
		strings.Contains(lowErr, "parameter `tools`")
}

// isPrefillThinkingError detects llama.cpp's rejection of assistant
// response prefill when enable_thinking is active on the server side.
func isPrefillThinkingError(err error) bool {
	if err == nil {
		return false
	}
	lowErr := strings.ToLower(err.Error())
	return strings.Contains(lowErr, "prefill") &&
		strings.Contains(lowErr, "thinking")
}

func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message {
	return proxy.NormalizeHistory(history, a.useNativeTools)
}

func (a *Agent) injectToolInstructions(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	instructions := prompts.BuildToolManual(info)
	newHistory := make([]proxy.Message, 0, len(history)+1)
	foundSystem := false
	for _, msg := range history {
		if !foundSystem && msg.Role == proxy.SystemRole {
			newMsg := msg
			newMsg.Content = prompts.InjectToolManual(newMsg.Content, instructions)
			newHistory = append(newHistory, newMsg)
			foundSystem = true
		} else {
			newHistory = append(newHistory, msg)
		}
	}
	if !foundSystem {
		newHistory = append([]proxy.Message{{
			Role:    proxy.SystemRole,
			Content: prompts.InjectToolManual("You are a powerful agentic AI.", instructions),
		}}, newHistory...)
	}
	return newHistory
}
