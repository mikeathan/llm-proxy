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
	// MaxSteps is increased to 25 to allow complex tasks to finish despite local model verbosity.
	MaxSteps = 25
	// AgentGlobalTimeout is the maximum duration for a complete agentic operation.
	AgentGlobalTimeout = 30 * time.Minute
	// AgentTurnTimeout is the maximum time allowed for a single LLM turn.
	AgentTurnTimeout = 10 * time.Minute
	// AgentRetryTimeout is the timeout used for fallback/retry logic.
	AgentRetryTimeout = 5 * time.Minute
)

// Agent represents a unified, stateful assistant that can use tools.
type Agent struct {
	client      proxy.Client
	provider    ToolProvider
	engine      Engine
	guardrails  *guardrails.GuardrailEngine
	logger      logging.Logger
	maxSteps    int
	observer    Observer
	workspaceID string
}

type AgentOptions struct {
	MaxSteps    int
	Logger      logging.Logger
	Guardrails  *guardrails.GuardrailEngine
	Observer    Observer
	WorkspaceID string
}

// NewAgent creates a new unified agent.
func NewAgent(client proxy.Client, provider ToolProvider, engine Engine, opts AgentOptions) *Agent {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = MaxSteps
	}
	if opts.Logger == nil {
		opts.Logger = logging.NewNopLogger()
	}
	// Default guardrail engine if none provided
	gr := opts.Guardrails
	if gr == nil {
		gr = guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil)
	}

	return &Agent{
		client:      client,
		provider:    provider,
		engine:      engine,
		guardrails:  gr,
		logger:      opts.Logger,
		maxSteps:    opts.MaxSteps,
		observer:    opts.Observer,
		workspaceID: opts.WorkspaceID,
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

	for steps < a.maxSteps {
		steps++
		if err := execCtx.Err(); err != nil {
			return "", currentHistory, fmt.Errorf("agent execution halted: %w", err)
		}

		a.notifyStepStart(steps)
		a.notifyThinking()

		var turnMsg proxy.Message
		err := func() error {
			turnCtx, turnCancel := context.WithTimeout(execCtx, AgentTurnTimeout)
			defer turnCancel()

			toolsList, err := a.provider.ListTools(turnCtx)
			if err != nil {
				return fmt.Errorf("failed to list tools: %w", err)
			}

			// PHYSICAL SIEVE: Prune middle history when nearing context limits
			totalChars := 0
			for _, m := range currentHistory {
				totalChars += len(m.Content)
			}
			if totalChars > 15000 {
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

					// Reset repetition detector memory on sieve activation
					recentCalls = recentCalls[:0]

					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: "SYSTEM: CRITICAL - Context window full. History pruned. Continue your task and finalize when ready.",
					})
				}
			}

			msg, err := a.computeNextResponse(turnCtx, currentHistory, toolsList)
			if err != nil {
				return err
			}

			// DIAGNOSTIC LOGGING: Capture the raw response from the model
			a.logger.Debug("raw model response received", 
				"content_length", len(msg.Content),
				"content_preview", strings.ReplaceAll(msg.Content, "\n", " "),
			)

			// Extract tools and CLEAN the message content to save tokens
			a.handleContentToolCalls(&msg)
			turnMsg = msg

			if len(turnMsg.ToolCalls) > 0 {
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

				for _, tc := range turnMsg.ToolCalls {
					key := toolKey{tc.Function.Name, tc.Function.Arguments}
					if tc.Function.Name != models.ToolSubmitFinalAnswer && tc.Function.Name != models.ToolSystemError {
						if len(recentCalls) > 0 && recentCalls[len(recentCalls)-1] == key {
							a.logger.Warn("duplicate action detected", "tool", key.name)
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

			// Agnostic Sentence Guard (Soft Exit)
			if isAutomation {
				trimmed := strings.TrimSpace(strings.ToLower(turnMsg.Content))
				hasFinality := false
				for _, s := range []string{".", "!", "?", "}", "```"} {
					if strings.HasSuffix(trimmed, s) {
						hasFinality = true
						break
					}
				}
				isSuspect := strings.Contains(trimmed, "<tool") || strings.Contains(trimmed, "\"tool\":")
				isReport := strings.Contains(trimmed, "task complete") ||
					strings.Contains(trimmed, "summary") ||
					strings.Contains(trimmed, "final report")

				if hasFinality && !isSuspect && isReport && a.isFinalReport(turnMsg.Content) {
					a.logger.Info("Agnostic Heuristic Soft Exit Triggered")
					return turnMsg.Content, currentHistory, nil
				}

				a.logger.Warn("turn resulted in no action - nagging model", "step", steps)
				currentHistory = append(currentHistory, proxy.Message{
					Role:    proxy.UserRole,
					Content: prompts.AutomationNagPrompt,
				})
				continue
			}

			if !isAutomation {
				if steps == 1 {
					return turnMsg.Content, currentHistory, nil
				}
				if a.countConsecutiveChat(currentHistory) >= 2 {
					return turnMsg.Content, currentHistory, nil
				}
			}
		}
	}
	return "", currentHistory, fmt.Errorf("agent exceeded max steps (%d)", a.maxSteps)
}

func (a *Agent) handleContentToolCalls(msg *proxy.Message) {
	if msg.Content != "" {
		// Do not overwrite msg.Content with cleaned content.
		// Keeping the XML in history is CRITICAL for local models to maintain their pattern.
		_, calls, ok := proxy.ParseContentToolCalls(msg.Content)
		if ok {
			msg.ToolCalls = append(msg.ToolCalls, calls...)
			a.logger.Debug("detected embedded tool calls in content", "count", len(calls))
		} else if strings.Contains(msg.Content, "{") || strings.Contains(msg.Content, "<tool") {
			// LOGGING ENHANCEMENT: Log suspicious but failed parsing attempts
			a.logger.Warn("tool call detection failed on suspicious content", 
				"content_snippet", strings.ReplaceAll(msg.Content, "\n", " "),
			)
		}
	}
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
	if !a.provider.UseNativeTools() {
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

	req := proxy.ChatRequest{
		Messages: prepared,
		Tools:    llmTools,
	}

	ch, err := a.client.Stream(ctx, req)
	if err != nil {
		a.logger.Warn("streaming not supported or failed, falling back to non-streaming", "error", err)
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := a.processStream(ctx, ch, &fullMsg); err != nil {
		return proxy.Message{}, err
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
	if !a.provider.UseNativeTools() {
		llmTools = nil
	}

	preparedHistory := a.prepareMessages(history)
	if llmTools == nil && len(tools) > 0 {
		preparedHistory = a.injectToolInstructions(preparedHistory, tools)
	}

	req := proxy.ChatRequest{
		Messages: preparedHistory,
		Tools:    llmTools,
	}

	if rawReq, err := json.Marshal(req); err == nil {
		a.logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	}

	resp, err := a.client.Chat(chatCtx, req)
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

	return resp.Choices[0].Message, nil
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

func (a *Agent) countRetries(history []proxy.Message) int {
	count := 0
	for _, h := range history {
		if h.Role == "user" && (strings.Contains(h.Content, "incomplete response") || strings.Contains(h.Content, "empty response")) {
			count++
		}
	}
	return count
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
		if tc.Type != "function" {
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

		if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
			a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
			a.notifyGuardrailViolation(tc.Function.Name, err)
			mu.Lock()
			a.appendToolResult(history, tc, formatGuardrailError(err))
			mu.Unlock()
			return nil
		}

		a.notifyToolCall(tc)
		toolCtx := models.WithWorkspaceID(ctx, a.workspaceID)
		result, err := a.engine.ExecuteTool(toolCtx, tc)
		mu.Lock()
		a.appendToolResult(history, tc, result)
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

func (a *Agent) isFinalReport(content string) bool {
	c := strings.ToLower(content)
	markers := []string{
		"# summary",
		"## summary",
		"### summary",
		"final answer:",
		"task complete",
		"### findings",
		"### task accomplished",
		"i have completed",
	}

	for _, m := range markers {
		if strings.Contains(c, m) {
			return len(content) > 50 && strings.Count(c, "```")%2 == 0
		}
	}
	return strings.Count(content, "###") >= 2 &&
		!strings.Contains(content, "```json") &&
		len(content) > 100 &&
		strings.Count(c, "```")%2 == 0
}

func formatToolError(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
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

func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message {
	return proxy.NormalizeHistory(history, a.provider.UseNativeTools())
}

func isFuzzyDuplicate(s1, s2 string) bool {
	if s1 == s2 {
		return true
	}
	if len(s1) == 0 || len(s2) == 0 {
		return false
	}
	if strings.HasPrefix(s1, s2) || strings.HasPrefix(s2, s1) {
		return true
	}
	w1 := strings.Fields(s1)
	w2 := strings.Fields(s2)
	if len(w1) == 0 || len(w2) == 0 {
		return false
	}
	matchCount := 0
	wordMap := make(map[string]bool)
	for _, w := range w1 {
		wordMap[w] = true
	}
	for _, w := range w2 {
		if wordMap[w] {
			matchCount++
		}
	}
	overlap := float64(matchCount) / float64(len(w1))
	return overlap > 0.7
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
