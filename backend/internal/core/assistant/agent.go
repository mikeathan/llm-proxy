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
	// MaxSteps is the maximum number of turns before the agent gives up.
	MaxSteps = 15
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

			// Open Claw v3 Phase 4: Token Pressure Injection
			totalChars := 0
			for _, m := range currentHistory {
				totalChars += len(m.Content)
			}
			if totalChars > 14000 { // 85% of 16,000 budget
				a.logger.Warn("critical context pressure detected", "chars", totalChars)
				currentHistory = append(currentHistory, proxy.Message{
					Role:    proxy.UserRole,
					Content: "SYSTEM: CRITICAL - Context window is almost full. You must complete your task and call submit_final_answer in your next response.",
				})
			}

			msg, err := a.computeNextResponse(turnCtx, currentHistory, toolsList)
			if err != nil {
				// Open Claw v3 Phase 4: Graceful Recovery on 400 Context Errors
				if strings.Contains(err.Error(), "context size exceeded") || strings.Contains(err.Error(), "400") {
					a.logger.Error("context limit reached - attempting graceful termination")
					if len(currentHistory) > 0 {
						lastMsg := currentHistory[len(currentHistory)-1]
						if a.isFinalReport(lastMsg.Content) {
							return fmt.Errorf("TASK_SUBMITTED_TEXT")
						}
					}
				}
				return err
			}

			a.handleContentToolCalls(&msg)
			msg.Content = normalizeContent(msg.Content)
			turnMsg = msg

			// Process tool calls within the turn context if they exist
			if len(turnMsg.ToolCalls) > 0 {
				// 1. Loop & Duplicate Detection
				for _, tc := range turnMsg.ToolCalls {
					key := toolKey{tc.Function.Name, tc.Function.Arguments}

					// Open Claw v2 Phase 4: If it matches the EXACT previous call, do not execute.
					// EXCEPTION: Always allow submit_final_answer and system_error to repeat to avoid meta-loops.
					if tc.Function.Name != models.ToolSubmitFinalAnswer && tc.Function.Name != models.ToolSystemError {
						if len(recentCalls) > 0 && recentCalls[len(recentCalls)-1] == key {
							a.logger.Warn("duplicate action detected", "tool", key.name)
							currentHistory = append(currentHistory, turnMsg)
							a.notify(EventMessage, turnMsg)
							
							// Inject observation directly into history
							currentHistory = append(currentHistory, proxy.Message{
								Role:    proxy.UserRole,
								Content: "SYSTEM WARNING: You just repeated an action with the exact same arguments. It did not progress the task or produced an error. Analyze the problem and try a completely different approach or tool.",
							})
							return nil // Continue the loop with the warning
						}

						if len(recentCalls) >= 3 {
							recentCalls = recentCalls[1:]
						}
						recentCalls = append(recentCalls, key)
					}
				}

				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
				err := a.processToolCalls(turnCtx, turnMsg, &currentHistory)
				if err != nil {
					return err
				}

				// Check for submission
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
			if err.Error() == "TASK_SUBMITTED_TEXT" {
				return currentHistory[len(currentHistory)-1].Content, currentHistory, nil
			}
			return "", currentHistory, err
		}

		// Handle termination logic only when no tools are called
		if len(turnMsg.ToolCalls) == 0 {
			// 1. Check for premature termination (empty responses)
			if a.isPrematureTermination(turnMsg, currentHistory) {
				retries := a.countRetries(currentHistory)
				if retries >= 2 {
					return turnMsg.Content, currentHistory, fmt.Errorf("model repeatedly returned incomplete responses")
				}
				if retries == 0 {
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: "You returned an incomplete response. You MUST continue using tools or reply with the final comprehensive Markdown report as requested.",
					})
					continue
				}
				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
				a.notifyPrematureTerminationNag(&currentHistory)
				continue
			}

			// 2. Append the response to history
			if len(currentHistory) == 0 || currentHistory[len(currentHistory)-1].Content != turnMsg.Content {
				currentHistory = append(currentHistory, turnMsg)
				a.notify(EventMessage, turnMsg)
			}

			// 3. Stop immediately on first turn if no tools are called (direct answer),
			// unless this is an explicit automation task which requires tool execution.
			isAutomation := false
			for _, m := range history {
				if prompts.IsAutomationTask(m.Content) {
					isAutomation = true
					break
				}
			}

			if steps == 1 && !isAutomation {
				return turnMsg.Content, currentHistory, nil
			}

			// Handle text-only turns in automation mode
			if isAutomation && len(turnMsg.ToolCalls) == 0 {
				// If the model is clearly providing a final report but forgot the tool call, 
				// allow it to terminate successfully instead of nagging it into a loop.
				if a.isFinalReport(turnMsg.Content) {
					a.logger.Info("accepting text-only final report as automation conclusion")
					return turnMsg.Content, currentHistory, nil
				}

				a.logger.Warn("automation model failed to output tool")

				// 1. APPEND the reasoning turn to history so the model knows it already thought about it.
				// But check for a duplicate reasoning loop first.
				isDuplicate := false
				if n := len(currentHistory); n >= 2 {
					lastMsg := currentHistory[n-1]
					// If last message was a nag and the one before was the SAME assistant content (fuzzy)
					if lastMsg.Role == proxy.UserRole && strings.Contains(lastMsg.Content, "SYSTEM ERROR") {
						prevAssistant := currentHistory[n-2]
						if prevAssistant.Role == proxy.AssistantRole && isFuzzyDuplicate(prevAssistant.Content, turnMsg.Content) {
							isDuplicate = true
						}
					}
				}

				if !isDuplicate {
					if len(currentHistory) == 0 || currentHistory[len(currentHistory)-1].Content != turnMsg.Content {
						currentHistory = append(currentHistory, turnMsg)
						a.notify(EventMessage, turnMsg)
					}
				}

				// 2. Standard single nag — replace if already present or escalate if duplicate
				nagContent := prompts.AutomationNagPrompt
				if isDuplicate {
					nagContent = prompts.AutomationDuplicateNagPrompt
				}

				if n := len(currentHistory); n > 0 && currentHistory[n-1].Role == proxy.UserRole && strings.Contains(currentHistory[n-1].Content, "SYSTEM ERROR") {
					currentHistory[n-1].Content = nagContent
				} else {
					currentHistory = append(currentHistory, proxy.Message{
						Role:    proxy.UserRole,
						Content: nagContent,
					})
				}
				continue
			}

			// 4. Stop if it's a final report (fallback for regular chats)
			if !isAutomation && a.isFinalReport(turnMsg.Content) {
				return turnMsg.Content, currentHistory, nil
			}

			// 5. Soft Termination: allow one more turn to ensure model is truly done
			// Check if we already have 2 consecutive chat messages with no tools.
			// IMPORTANT: For automations, we ignore this and continue until submit_final_answer or maxSteps.
			if isAutomation {
				continue
			}

			chatCount := 0
			for j := len(currentHistory) - 1; j >= 0; j-- {
				if currentHistory[j].Role == proxy.AssistantRole && len(currentHistory[j].ToolCalls) == 0 {
					chatCount++
				} else {
					break
				}
			}
			if chatCount < 2 {
				continue
			}
			return turnMsg.Content, currentHistory, nil
		}

	}
	return "", currentHistory, fmt.Errorf("agent exceeded max steps (%d) without reaching a conclusion", a.maxSteps)
}

func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	llmTools := tools
	if !a.provider.UseNativeTools() {
		llmTools = nil
	}

	prepared := a.prepareMessages(history)

	// Always inject the text-based tool manual for non-native mode.
	// Also inject for automation tasks even when native tools are active:
	// many models receive API-level tools fine but fail to emit native tool
	// calls, so providing the XML fallback in text ensures they always have
	// a format they can output.
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

	// If the response is empty and we were using native tools, the model might be paralyzed by the 'tools' parameter.
	// Fallback to prompt-based tool calling (no native tools).
	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 && llmTools != nil {
		a.logger.Info("empty response with native tools, retrying without them")
		return a.computeNextResponseNonStreaming(ctx, history, nil)
	}

	// If the stream was empty, fallback to non-streaming which often provides better error diagnostics.
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

				// Accumulate Content
				chunkContent := choice.Delta.Content
				if chunkContent == "" && choice.Message.Content != "" {
					chunkContent = choice.Message.Content
				}

				// Capture reasoning content from R1-style models that map <think> blocks to reasoning_content
				reasoningChunk := choice.Delta.ReasoningContent
				if reasoningChunk == "" && choice.Message.ReasoningContent != "" {
					reasoningChunk = choice.Message.ReasoningContent
				}

				totalChunk := chunkContent + reasoningChunk

				if totalChunk != "" {
					fullMsg.Content += totalChunk

					// UI Smoothing: If we detect the start of a tool call signature,
					// stop streaming to the text-content bus to avoid showing raw technical markup.
					displayContent, hasToolCall := FilterStreamingMarkup(fullMsg.Content)

					// If we've reached a tool call, provide a small visual hint in the stream
					// so the user knows the agent is still active but is now processing technical steps.
					if hasToolCall {
						a.notify(EventToolStream, displayContent+"\n\n🛠️ *Agent is initiating tool calls...*")
					} else {
						a.notify(EventToolStream, displayContent)
					}
				}

				// Accumulate Tool Calls
				if len(choice.Delta.ToolCalls) > 0 {
					for _, tc := range choice.Delta.ToolCalls {
						// Stream-based tool calls often come in chunks
						if tc.ID != "" {
							fullMsg.ToolCalls = append(fullMsg.ToolCalls, tc)
						} else if len(fullMsg.ToolCalls) > 0 {
							// Append arguments to the last tool call
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

	// Use prepared history to avoid prefill errors and maintain consistency.
	preparedHistory := a.prepareMessages(history)
	
	// Ensure we inject instructions for non-streaming as well if native tools are off
	if llmTools == nil && len(tools) > 0 {
		preparedHistory = a.injectToolInstructions(preparedHistory, tools)
	}

	req := proxy.ChatRequest{
		Messages: preparedHistory,
		Tools:    llmTools,
	}

	if rawReq, err := json.Marshal(req); err == nil {
		a.logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	} else {
		a.logger.Error("Failed to marshal LLM Non-Stream Request", "error", err)
	}

	resp, err := a.client.Chat(chatCtx, req)

	if err != nil && isToolSupportError(err) {
		a.logger.Warn("model does not support tools, retrying without them", "error", err)
		a.notifyFallbackWarning(err)

		// Try again without tools
		chatCtx2, cancel2 := context.WithTimeout(ctx, AgentRetryTimeout)
		defer cancel2()
		resp, err = a.client.Chat(chatCtx2, proxy.ChatRequest{
			Messages: history,
		})
	}

	if err != nil {
		return proxy.Message{}, fmt.Errorf("llm completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return proxy.Message{}, fmt.Errorf("llm returned no choices")
	}

	return resp.Choices[0].Message, nil
}

func (a *Agent) handleContentToolCalls(msg *proxy.Message) {
	if msg.Content != "" {
		if _, calls, ok := proxy.ParseContentToolCalls(msg.Content); ok {
			msg.ToolCalls = append(msg.ToolCalls, calls...)
			// Note: We no longer overwrite msg.Content with cleanedContent.
			// This ensures the user can see the model's full reasoning and the tool tags in the UI.
			a.logger.Debug("detected embedded tool calls in content", "count", len(calls))
		}
	}
}

func (a *Agent) isPrematureTermination(msg proxy.Message, history []proxy.Message) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		// If there are tool calls, it's definitely NOT a termination.
		if len(msg.ToolCalls) > 0 {
			return false
		}
		return true
	}

	// Basic repetition detection for short content-only responses
	// if the same message is returned 3 times in a row, it's premature.
	if len(history) >= 2 {
		last := history[len(history)-1]
		if last.Role == proxy.AssistantRole && strings.TrimSpace(last.Content) == content {
			// Found one repeat, check if it's a pattern
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

	// 1. Submission Isolation: If submit_final_answer is called, it MUST be the only tool call.
	// This prevents "greedy batching" where a model submits success before seeing if previous tools failed.
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
		// We return a specialized error to the model asking it to separate the submission
		errorMsg := prompts.AutomationRejectedSubmissionPrompt
		// We append this result to all calls in the turn to be safe
		for _, tc := range msg.ToolCalls {
			a.appendToolResult(history, tc, map[string]string{"error": errorMsg})
		}
		mu.Unlock()
		return nil // We return nil to continue the loop and let the model see the rejection
	}

	for _, tc := range msg.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}

		if tc.Type != "function" {
			continue
		}

		a.logger.Info("agent attempting tool execution", "name", tc.Function.Name, "args", tc.Function.Arguments)

		// 2. Generic Schema Validation
		toolsList, _ := a.provider.ListTools(ctx)
		if err := validateToolArgs(tc, toolsList); err != nil {
			a.logger.Warn("tool argument validation failed", "name", tc.Function.Name, "error", err)
			mu.Lock()
			a.appendToolResult(history, tc, map[string]string{"error": fmt.Sprintf("INVALID ARGUMENTS: %v", err)})
			mu.Unlock()
			return nil
		}

		// 3. Validate against guardrails
		if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
			a.logger.Warn("guardrail check rejected tool call", "name", tc.Function.Name, "error", err)
			a.notifyGuardrailViolation(tc.Function.Name, err)

			mu.Lock()
			a.appendToolResult(history, tc, formatGuardrailError(err))
			mu.Unlock()
			// For coding tasks, we STOP the batch on any guardrail violation
			return nil 
		}

		// 3. Notify start
		a.notifyToolCall(tc)

		// 4. Execute tool
		toolCtx := models.WithWorkspaceID(ctx, a.workspaceID)
		result, err := a.engine.ExecuteTool(toolCtx, tc)
		
		// 5. Append result and notify
		mu.Lock()
		a.appendToolResult(history, tc, result)
		a.notifyToolResult(tc.ID, tc.Function.Name, result)
		mu.Unlock()

		if err != nil {
			a.logger.Warn("tool execution failed - stopping batch", "name", tc.Function.Name, "error", err)
			// STOP-ON-ERROR: If a tool fails, we stop the batch immediately.
			// This forces the model to observe the error and fix it before proceeding.
			return nil
		}

		// 6. Check for early termination if submit_final_answer was called
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
			// Also check for significant length to avoid false positives on short sentences
			return len(content) > 300
		}
	}
	
	// Fallback: If it contains a lot of Markdown structure and no tool tags
	return strings.Count(content, "###") >= 2 && !strings.Contains(content, "```json")
}

func formatToolError(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

func formatGuardrailError(err error) map[string]string {
	return map[string]string{"error": "Guardrail violation: " + err.Error()}
}

// extractTaskSummary dynamically pulls a human-readable summary from the submit_final_answer arguments.
func extractTaskSummary(rawArgs string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "Task complete."
	}

	// 1. Try known keys first for precision
	for _, key := range []string{"summary", "message", "report", "findings", "content", "result"} {
		if val, ok := args[key].(string); ok && val != "" {
			return val
		}
	}

	// 2. Dynamic Fallback: Use the first non-empty string field found in the map
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

	// Parse parameters
	params, ok := targetTool.Function.Parameters.(map[string]any)
	if !ok {
		return nil // No parameters to validate
	}

	requiredRaw, ok := params["required"]
	if !ok {
		return nil // No required fields
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

	// Parse arguments
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return fmt.Errorf("failed to parse arguments as JSON: %w", err)
	}

	for _, field := range required {
		val, ok := args[field]
		if !ok {
			return fmt.Errorf("missing required parameter '%s'", field)
		}
		
		// String fields shouldn't be empty
		if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf("parameter '%s' cannot be empty", field)
		}
	}

	return nil
}

// appendToolResult helper with memory-sensitive pruning
func (a *Agent) appendToolResult(history *[]proxy.Message, tc proxy.ToolCall, result any) {
	raw, _ := json.Marshal(result)
	strContent := string(raw)

	// Open Claw v2: Proactively truncate large tool results to protect context window.
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

// prepareMessages adapts history for the LLM request using unified normalization rules.
func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message {
	return proxy.NormalizeHistory(history, a.provider.UseNativeTools())
}

// isFuzzyDuplicate returns true if s1 and s2 are highly similar,
// indicating the model is repeating itself with slight variations.
func isFuzzyDuplicate(s1, s2 string) bool {
	if s1 == s2 {
		return true
	}
	// Simple length-based and prefix-based fuzzy check
	// If the strings are almost the same length and share a significant prefix
	if len(s1) == 0 || len(s2) == 0 {
		return false
	}
	
	// Check if one is a prefix of the other (common in looping reasoning)
	if strings.HasPrefix(s1, s2) || strings.HasPrefix(s2, s1) {
		return true
	}

	// Calculate word overlap
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

	// If more than 70% of words overlap, consider it a duplicate
	overlap := float64(matchCount) / float64(len(w1))
	return overlap > 0.7
}

// injectToolInstructions prepends tool definitions to the first system message.
func (a *Agent) injectToolInstructions(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}

	// Map to prompts.ToolInfo for template building
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}

	instructions := prompts.BuildToolManual(info)

	// Find the first system message or prepend a new one
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

