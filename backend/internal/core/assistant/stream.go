// stream.go — All LLM streaming and non-streaming calls, stream processing
// (chunk accumulation, stuck detection, tool-call extraction from reasoning),
// budget pre-flight, prefill logic, and fallback chains (native→XML→non-streaming).
// Also holds FilterStreamingMarkup and normalizeContent (moved from content_filter.go).
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/memory"
)

const (
	streamReasoningBudgetDivisor  = 4           // reasoning_budget = max_tokens / 4 — caps thinking without cutting complex tool calls
	streamHeartbeatInterval       = 30 * time.Second  // progress log during long streams
	nonStreamHeartbeatInterval    = 15 * time.Second  // fallback_waiting lifecycle event
	automationTemperature         = 0.1         // low temperature for deterministic automation tasks
	stuckThresholdMultiplier      = 2           // stuck threshold = max_tokens * 2
)

var (
	toolCallInContent = regexp.MustCompile(`(?si)<tool_call>.*?</tool_call>`)
	extractionRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?s)\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"](.*?)['"]\s*\}?\s*\]?`),
		regexp.MustCompile(`(?s)\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"]([^'"]*)`),
	}
)

func (a *Agent) buildChatRequest(
	prepared []proxy.Message,
	llmTools []proxy.Tool,
	isAutomationCtx bool,
) proxy.ChatRequest {
	req := proxy.ChatRequest{
		Messages:  prepared,
		Tools:     llmTools,
		MaxTokens: a.maxTokens,
	}
	if a.useNativeTools && isAutomationCtx {
		req.ToolChoice = proxy.ToolChoiceRequired
	}
	if isAutomationCtx {
		req.Temperature = automationTemperature
		if a.reasoningBudget > 0 {
			req.SetReasoningBudget(a.reasoningBudget)
		} else {
			a.reasoningBudget = a.maxTokens / streamReasoningBudgetDivisor
			req.SetReasoningBudget(a.reasoningBudget)
		}
	}
	return req
}

func (a *Agent) prepareMessagesForTurn(
	history []proxy.Message,
	tools []proxy.Tool,
	llmTools []proxy.Tool,
) ([]proxy.Message, string) {
	prepared := a.prepareMessages(history)
	if llmTools == nil && len(tools) > 0 {
		prepared = a.injectToolInstructions(prepared, tools)
	} else if llmTools != nil && len(tools) > 0 {
		prepared = a.injectNativeToolReference(prepared, tools)
	}

	// Skip memory injection for automation tasks.  The memory search uses the
	// full task prompt (hundreds of words) as the query, which returns generic
	// irrelevant entries (tool_versions, system_os_info, previous run outcomes)
	// via FTS5 broad-match.  The <memory> block is appended at the END of
	// prepared messages — the most salient position for the model — so it
	// overwrites the finalization instruction ("call submit_final_answer")
	// in the model's attention.  For a 4B model at step 10 with 18+ turns of
	// history, this pushes the model to call notify_user instead.
	// See docs/audits/memory-injection-investigation.md
	isAutoCtx := a.findAutomationCtx(history)
	if !isAutoCtx {
		prepared = a.injectActiveMemory(prepared, history)
	}

	var prefill string
	if a.shouldPrefill(isAutoCtx) {
		prefill = prompts.AutomationPrefline
		prepared = append(prepared, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: prefill,
		})
	}
	return prepared, prefill
}

// injectActiveMemory searches for relevant memories using the last user
// message (or cached automation task prompt) as query, and appends the
// result as a separate system message wrapped in <memory> tags at the end
// of prepared messages.  This runs ONCE per session (first turn only) to
// avoid per-turn noise.  Memories saved mid-session become visible on the
// next session.  See docs/audits/memory-injection-investigation.md
// for the rationale and alternatives tried.
func (a *Agent) injectActiveMemory(prepared []proxy.Message, history []proxy.Message) []proxy.Message {
	if a.memoryStore == nil {
		return prepared
	}

	if a.memoryInjected {
		return prepared
	}
	a.memoryInjected = true

	lastUserMsg := ""
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.UserRole {
			c := history[i].Content
			if strings.HasPrefix(c, "SYSTEM") || strings.HasPrefix(c, "REJECTED") {
				continue
			}
			if strings.Contains(c, "conversation history is about to be compressed") {
				continue
			}
			if strings.Contains(c, "Stop analyzing") || strings.Contains(c, "Stop writing") {
				continue
			}
			lastUserMsg = c
			break
		}
	}
	if lastUserMsg == "" {
		return prepared
	}

	if a.automationTaskPrompt == "" {
		for _, msg := range history {
			if msg.Role == proxy.UserRole && strings.Contains(msg.Content, prompts.AutomationMarker) {
				a.automationTaskPrompt = msg.Content
				break
			}
		}
	}

	query := lastUserMsg
	if a.automationTaskPrompt != "" {
		query = a.automationTaskPrompt
	}

	ctx := context.Background()
	entries, err := a.memoryStore.Search(ctx, a.workspaceID, query, 5)
	profileEntries, peErr := a.memoryStore.List(ctx, a.workspaceID, memory.UserProfile, 20, 0)

	if len(entries) > 0 {
		titles := make([]string, len(entries))
		for i, e := range entries {
			titles[i] = e.Title
		}
		a.logger.Debug("memory injection", "query", query, "titles", titles)
	}

	if (err != nil || len(entries) == 0) && (peErr != nil || len(profileEntries) == 0) {
		return prepared
	}

	var b strings.Builder

	if len(entries) > 0 {
		b.WriteString(prompts.RelevantMemoriesHeader)
		for i, e := range entries {
			if i > 0 {
				b.WriteString("---\n")
			}
			b.WriteString("**")
			b.WriteString(e.Title)
			b.WriteString("**: ")
			b.WriteString(e.Content)
			b.WriteString("\n")
		}
		if meter := memoryUsageMeter(ctx, a.memoryStore, a.workspaceID); meter != "" {
			b.WriteString(meter)
		}
		b.WriteString(prompts.RelevantMemoriesFooter)
	}

	if len(profileEntries) > 0 {
		b.WriteString("\n")
		b.WriteString(prompts.UserProfileHeader)
		for i, e := range profileEntries {
			if i > 0 {
				b.WriteString("---\n")
			}
			b.WriteString("**")
			b.WriteString(e.Title)
			b.WriteString("**: ")
			b.WriteString(e.Content)
			b.WriteString("\n")
		}
		b.WriteString(prompts.UserProfileFooter)
	}

	memBlock := b.String()
	runes := []rune(memBlock)
	if len(runes) > 2500 {
		memBlock = string(runes[:2500]) + "\n...[truncated]"
		if strings.Contains(memBlock, "<relevant_memories>") && !strings.Contains(memBlock, "</relevant_memories>") {
			memBlock += prompts.RelevantMemoriesFooter
		}
		if strings.Contains(memBlock, "<user_profile>") && !strings.Contains(memBlock, "</user_profile>") {
			memBlock += prompts.UserProfileFooter
		}
	}
	if memBlock == "" {
		return prepared
	}

	// Append memory block as a separate system message at the end of prepared
	// messages, right before the current user turn.  This keeps the system
	// prompt + state + history KV cache stable — only this final message
	// changes each turn.
	prepared = append(prepared, proxy.Message{
		Role:    proxy.SystemRole,
		Content: "<memory>\n" + memBlock + "\n</memory>",
	})

	a.notifyMemoryRecall(lastUserMsg, len(entries)+len(profileEntries))
	return prepared
}

func (a *Agent) doPreflightCheck(
	ctx context.Context,
	history []proxy.Message,
	req *proxy.ChatRequest,
) (txnID string, err error) {
	if a.orch == nil || a.orch.Budget == nil {
		return "", nil
	}
	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
	}
	preflight, pfErr := a.orch.Budget.PreFlightCheck(ctx, a.workspaceID,
		orchestrator.PreFlightRequest{
			ModelName:       a.modelName,
			ProviderType:    a.providerType,
			ContextChars:    totalChars,
			MaxTokens:       a.maxTokens,
			ReasoningBudget: a.reasoningBudget,
			ICUWeight:       a.icuWeight,
		})
	if pfErr != nil {
		return "", fmt.Errorf("budget error: %w", pfErr)
	}
	if !preflight.Allowed {
		return "", fmt.Errorf("budget exceeded: %s", preflight.Reason)
	}
	if preflight.SqueezeFactor < 1.0 {
		a.maxTokens = preflight.AdjustedMaxTokens
		a.reasoningBudget = preflight.AdjustedReasoning
		req.MaxTokens = a.maxTokens
	}
	return preflight.TransactionID, nil
}

func (a *Agent) handlePrefillRejection(
	ctx context.Context, history []proxy.Message, tools []proxy.Tool,
) (<-chan *proxy.ChatResponse, error) {
	a.prefillDisabled = true
	a.notifyPrefillDisabled()
	prepared := a.prepareMessages(history)
	if len(tools) > 0 {
		prepared = a.injectToolInstructions(prepared, tools)
	}
	req := proxy.ChatRequest{
		Messages:        prepared,
		Tools:           nil,
		MaxTokens:       a.maxTokens,
		Temperature:     automationTemperature,
		ReasoningBudget: a.maxTokens / streamReasoningBudgetDivisor,
	}
	return a.client.Stream(ctx, req)
}

func (a *Agent) handleEmptyStream(
	ctx context.Context, history []proxy.Message,
	tools []proxy.Tool, llmTools []proxy.Tool,
) (proxy.Message, error) {
	history = injectRetryContext(history)
	if llmTools != nil {
		a.logger.Info("empty response with native tools, retrying in XML mode")
		a.notifyLifecycle("fallback_started", map[string]any{
			"reason": "empty stream with native tools", "mode": "xml",
		})
		savedNative := a.useNativeTools
		savedSuppress := a.suppressReasoningBudget
		a.useNativeTools = false
		a.suppressReasoningBudget = true
		msg, err := a.computeNextResponseStreamXML(ctx, history, tools)
		a.useNativeTools = savedNative
		a.suppressReasoningBudget = savedSuppress
		return msg, err
	}
	return a.computeNextResponseNonStreaming(ctx, history, tools)
}

// computeNextResponse tries streaming first, with fallback to non-streaming
// or XML-mode streaming on failure.  streamErr reuse across the deferred refund
// closure and subsequent retries is intentional (shadow-free single var).
func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	llmTools := tools
	if !a.useNativeTools {
		llmTools = nil
	}

	prepared, prefill := a.prepareMessagesForTurn(history, tools, llmTools)
	isAutoCtx := a.findAutomationCtx(history)
	req := a.buildChatRequest(prepared, llmTools, isAutoCtx)

	txnID, pfErr := a.doPreflightCheck(ctx, history, &req)
	if pfErr != nil {
		return proxy.Message{}, pfErr
	}

	ch, streamErr := a.client.Stream(ctx, req)
	a.logger.Info("stream request sent", "model", a.modelName,
		"max_tokens", a.maxTokens, "tool_choice", req.ToolChoice)

	if a.orch != nil && a.orch.Budget != nil && txnID != "" {
		defer func() {
			if streamErr != nil {
				if refErr := a.orch.Budget.Refund(ctx, txnID); refErr != nil {
					a.logger.Warn("ICU refund failed", "txn", txnID, "error", refErr)
				} else {
					a.logger.Warn("ICU refunded due to stream failure", "txn", txnID)
				}
			}
		}()
	}

	if streamErr != nil {
		if prefill != "" && isPrefillThinkingError(streamErr) {
			a.logger.Info("prefill rejected by server, retrying without prefill")
			ch, streamErr = a.handlePrefillRejection(ctx, history, tools)
			prefill = ""
		}
		if streamErr != nil {
			a.logger.Warn("streaming not supported, falling back to non-streaming")
			a.memoryInjected = false  // retry injection on the non-streaming path
			return a.computeNextResponseNonStreaming(ctx, history, tools)
		}
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if streamErr = a.processStream(ctx, ch, &fullMsg); streamErr != nil {
		return proxy.Message{}, streamErr
	}

	if prefill != "" {
		fullMsg.Content = prefill + fullMsg.Content
	}

	reasons := len(fullMsg.ReasoningContent)
	if a.reasoningBudget > 0 && reasons > a.reasoningBudget*2 {
		a.logger.Warn("reasoning budget far exceeded — server may not be enforcing",
			"reasoning_budget", a.reasoningBudget,
			"reasoning_len", reasons,
			"model", a.modelName)
	}
	a.logger.Info("stream completed",
		"content_len", len(fullMsg.Content),
		"reasoning_len", reasons,
		"tool_calls", len(fullMsg.ToolCalls))

	if t := GetUsageTracker(ctx); t != nil {
		t.AddLLMCall(len(prepared), len(fullMsg.Content), len(fullMsg.ReasoningContent))
	}

	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 {
		return a.handleEmptyStream(ctx, history, tools, llmTools)
	}

	return fullMsg, nil
}

func injectRetryContext(history []proxy.Message) []proxy.Message {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.UserRole {
			history[i].Content = fmt.Sprintf("%s\n\n%s", prompts.RetrySignal, history[i].Content)
			break
		}
	}
	return history
}

// checkStreamStuck aborts early when reasoning content exceeds the threshold
// with no text or tool call output.  The threshold is scaled by
// reasoningBudget * 2 (in chars) so models that don't enforce the budget
// server-side are caught mid-stream; falls back to maxTokens * 2 when the
// reasoning budget is 0 (non-automation turns).
func (a *Agent) checkStreamStuck(fullMsg *proxy.Message) bool {
	if a.skipStuckCheck || len(fullMsg.Content) > 0 || len(fullMsg.ToolCalls) > 0 {
		return false
	}
	return len(fullMsg.ReasoningContent) > a.stuckThreshold()
}

// tryExtractToolCallFromReasoning handles models (e.g. Qwen 3.5) that emit
// <tool_call> blocks inside <think> reasoning content, where they're invisible
// to the native-tool parser but still valid XML.  Extracts and promotes.
func (a *Agent) tryExtractToolCallFromReasoning(fullMsg *proxy.Message) bool {
	if len(fullMsg.ReasoningContent) == 0 {
		return false
	}
	if !toolCallInContent.MatchString(fullMsg.ReasoningContent) {
		return false
	}
	cleaned := cleanReasoningContent(fullMsg.ReasoningContent)
	if cleaned == "" {
		return false
	}
	fullMsg.Content = cleaned
	return true
}

func (a *Agent) processStream(ctx context.Context, ch <-chan *proxy.ChatResponse, fullMsg *proxy.Message) error {
	var tokUsed, reasonUsed int
	var budgetWarned bool

	streamDone := make(chan struct{})
	defer close(streamDone)

	go func() {
		ticker := time.NewTicker(streamHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.logger.Info("stream still generating", "content_len", len(fullMsg.Content), "reasoning_len", len(fullMsg.ReasoningContent))
			case <-ctx.Done():
				return
			case <-streamDone:
				return
			}
		}
	}()

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

				if a.orch != nil && a.orch.Interceptor != nil {
					result := a.orch.Interceptor.InterceptChunk(orchestrator.StreamChunk{
						Content:          chunkContent,
						ReasoningContent: reasoningChunk,
						ProviderType:     a.providerType,
					})
					tokUsed += result.TokensUsed
					reasonUsed += result.ReasoningUsed

					if !a.suppressReasoningBudget {
						term := a.orch.Interceptor.InterceptChunkWithBudget(ctx,
							orchestrator.StreamChunk{},
							tokUsed, reasonUsed, a.maxTokens, a.reasoningBudget,
						)
						if term.ShouldTerminate && !budgetWarned {
							budgetWarned = true
							// Warn only — do NOT terminate. The server (llama.cpp)
							// enforces reasoning_budget at the API level by forcing
							// the model out of thinking mode. Terminating here would
							// kill the stream before the first content chunk arrives,
							// triggering the XML/non-streaming fallback chain. The
							// stuck detector (maxTokens × 2 chars) and maxTokens
							// budget provide sufficient protection. See AGENTS.md
							// pitfall #20 for the reasoning behind warn-only.
							a.logger.Warn("reasoning budget exceeded, letting server enforcement handle it",
								"tokens_used", tokUsed, "reasoning_used", reasonUsed,
								"token_budget", a.maxTokens, "reasoning_budget", a.reasoningBudget)
						}
					}
				}

				if a.tryExtractToolCallFromReasoning(fullMsg) {
					return nil
				}

				if a.checkStreamStuck(fullMsg) {
					a.logger.Warn("reasoning stuck detected, aborting stream early to trigger fallback", "reasoning_chars", len(fullMsg.ReasoningContent), "stuck_threshold", a.stuckThreshold())
					a.notifyLifecycle("stuck_detected", map[string]any{
						"reasoning_chars": len(fullMsg.ReasoningContent),
					})
					return nil
				}

				if chunkContent != "" {
					fullMsg.Content += chunkContent
				}
				if reasoningChunk != "" {
					fullMsg.ReasoningContent += reasoningChunk
				}
				if chunkContent != "" || reasoningChunk != "" {
					displayText := fullMsg.ReasoningContent + fullMsg.Content
					displayContent, hasToolCall := FilterStreamingMarkup(displayText)
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

func (a *Agent) retryWithoutTools(ctx context.Context, history []proxy.Message) (*proxy.ChatResponse, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentRetryTimeout)
	defer cancel()
	return a.client.Chat(chatCtx, proxy.ChatRequest{Messages: history, MaxTokens: a.maxTokens})
}

func (a *Agent) computeNextResponseNonStreaming(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()

	llmTools := tools
	if !a.useNativeTools {
		llmTools = nil
	}

	preparedHistory, prefill := a.prepareMessagesForTurn(history, tools, llmTools)

	isAutomationCtx := a.findAutomationCtx(history)
	req := a.buildChatRequest(preparedHistory, llmTools, isAutomationCtx)
	if isAutomationCtx && a.suppressReasoningBudget {
		req.SetReasoningBudget(0)
	}

	if rawReq, err := json.Marshal(req); err == nil {
		a.logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	}

	a.logger.Info("non-stream request sent", "model", a.modelName, "max_tokens", a.maxTokens, "tool_choice", req.ToolChoice, "temperature", req.Temperature, "reasoning_budget", req.ReasoningBudget)

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	heartbeatStart := time.Now()
	go func() {
		ticker := time.NewTicker(nonStreamHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(heartbeatStart).Round(time.Second)
				a.notifyLifecycle("fallback_waiting", map[string]any{
					"elapsed": elapsed.String(),
				})
			case <-ctx.Done():
				return
			case <-heartbeatDone:
				return
			}
		}
	}()

	resp, err := a.client.Chat(chatCtx, req)
	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.logger.Info("prefill rejected by server (thinking mode), retrying without prefill in XML mode (non-stream)")
		a.prefillDisabled = true
		a.notifyPrefillDisabled()
		prefill = ""
		preparedHistory = a.prepareMessages(history)
		if len(tools) > 0 {
			preparedHistory = a.injectToolInstructions(preparedHistory, tools)
		}
		req = proxy.ChatRequest{
			Messages:    preparedHistory,
			Tools:       nil,
			MaxTokens:   a.maxTokens,
			Temperature: automationTemperature,
		}
		req.SetReasoningBudget(a.maxTokens / streamReasoningBudgetDivisor)
		if a.suppressReasoningBudget {
			req.SetReasoningBudget(0)
		}
		resp, err = a.client.Chat(chatCtx, req)
	}
	if err != nil && isToolSupportError(err) {
		a.logger.Warn("model does not support tools, retrying without them", "error", err)
		a.notifyFallbackWarning(err)
		resp, err = a.retryWithoutTools(ctx, history)
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
	if t := GetUsageTracker(ctx); t != nil {
		inputTokens := 0
		for _, m := range preparedHistory {
			inputTokens += len(m.Content)
		}
		t.AddLLMCall(inputTokens, len(msg.Content), len(msg.ReasoningContent))
	}
	return msg, nil
}

func (a *Agent) computeNextResponseStreamXML(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()

	prepared, prefill := a.prepareMessagesForTurn(history, tools, nil)

	req := proxy.ChatRequest{
		Messages:    prepared,
		Tools:       nil,
		MaxTokens:   a.maxTokens,
		Temperature: automationTemperature,
	}

	a.logger.Info("xml stream retry sent", "model", a.modelName, "max_tokens", a.maxTokens, "prefill", prefill != "")

	ch, err := a.client.Stream(ctx, req)

	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.logger.Info("prefill rejected by server (thinking mode), retrying stream without prefill")
		a.prefillDisabled = true
		a.notifyPrefillDisabled()
		prefill = ""
		prepared, _ = a.prepareMessagesForTurn(history, tools, nil)
		req = proxy.ChatRequest{
			Messages:    prepared,
			Tools:       nil,
			MaxTokens:   a.maxTokens,
			Temperature: automationTemperature,
		}
		ch, err = a.client.Stream(ctx, req)
	}

	if err != nil {
		a.logger.Warn("xml stream retry failed, falling back to non-streaming", "error", err)
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	savedSkip := a.skipStuckCheck
	a.skipStuckCheck = true
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	streamErr := a.processStream(ctx, ch, &fullMsg)
	a.skipStuckCheck = savedSkip

	if streamErr != nil {
		return proxy.Message{}, streamErr
	}

	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 {
		saved := a.prefillDisabled
		a.prefillDisabled = true
		msg, err := a.computeNextResponseNonStreaming(ctx, history, tools)
		a.prefillDisabled = saved
		return msg, err
	}

	if prefill != "" {
		fullMsg.Content = prefill + fullMsg.Content
	}

	a.logger.Info("xml stream retry completed", "content_len", len(fullMsg.Content), "reasoning_len", len(fullMsg.ReasoningContent), "tool_calls", len(fullMsg.ToolCalls))
	return fullMsg, nil
}

func (a *Agent) findAutomationCtx(history []proxy.Message) bool {
	for _, m := range history {
		if prompts.IsAutomationTask(m.Content) {
			return true
		}
	}
	return false
}

func (a *Agent) shouldPrefill(isAutomationCtx bool) bool {
	return a.usePrefill && !a.prefillDisabled && isAutomationCtx && !a.useNativeTools
}

func (a *Agent) stuckThreshold() int {
	threshold := a.maxTokens * stuckThresholdMultiplier
	if threshold < MinReasoningStuckThreshold {
		return MinReasoningStuckThreshold
	}
	return threshold
}

func cleanReasoningContent(s string) string {
	cleaned := strings.ReplaceAll(s, "<think>", "")
	cleaned = strings.ReplaceAll(cleaned, "</think>", "")
	return strings.TrimSpace(cleaned)
}

func FilterStreamingMarkup(content string) (displayContent string, hasToolCall bool) {
	cutoffPatterns := []string{
		"<function-name>", "</function-name>", "<args-json-object>", "</args-json-object>",
		"<tools>", "functions.",
		"<|tool_call", "<tool_call",
		"[TOOL_CALLS]", "[ARGS]",
		"```json", "```",
		"{\"name\":", "[{\"name\":",
		"{\"target\":", "{\"mode\":", "{\"command\":",
		"[{'type':", "{\"type\":",
	}

	displayContent = content
	for _, p := range cutoffPatterns {
		if idx := strings.Index(displayContent, p); idx != -1 {
			displayContent = displayContent[:idx]
			hasToolCall = true
			break
		}
	}
	return displayContent, hasToolCall
}

func normalizeContent(content string) string {
	content = strings.TrimSpace(content)

	for _, re := range extractionRegexes {
		content = re.ReplaceAllString(content, "$1")
	}

	return strings.TrimSpace(content)
}
