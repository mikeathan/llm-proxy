// stream.go — All LLM streaming and non-streaming calls, stream processing
// (chunk accumulation, stuck detection, tool-call extraction from reasoning),
// budget pre-flight, prefill logic, and fallback chains (native→XML→non-streaming).
// Also holds FilterStreamingMarkup and normalizeContent (moved from content_filter.go).
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/memory"
)

const (
	// ⚠ DO NOT CHANGE THIS VALUE without running the full smoke test.
	// divisor=4 was tried and caused recompilation loops at late turns because
	// 682 tokens was too little for the model to plan the next step at turn 18+.
	// divisor=3 gives 910 tokens — enough headroom.  Going higher than 3 wastes
	// generation budget, going lower than 3 causes the planning-cutoff loop.
	// See docs/audits/memory-injection-investigation.md for the investigation.
	streamReasoningBudgetDivisor  = 3           // reasoning_budget = max_tokens / 3 — gives ~910 tokens for 2730 max_tokens, enough to review history and plan next tool call
	stuckNonReasoningDivisor      = 1           // early stuck threshold for non-reasoning models: maxTokens / divisor chars of pure reasoning triggers stuck. Divisor=1 gives threshold at maxTokens (e.g. 2048 for local models). Divisor=2 was too tight — Gemma 4 produces ~1371 chars of legitimate reasoning before outputting, causing false positives. Divisor=1 catches stuck 2x faster than the pre-change baseline (maxTokens*2) while giving reasoning-capable models room. See docs/audits/write-file-truncation-cycles.md.
	streamHeartbeatInterval       = 30 * time.Second  // progress log during long streams
	nonStreamHeartbeatInterval    = 15 * time.Second  // fallback_waiting lifecycle event
	stuckThresholdMultiplier      = 2           // stuck threshold = max_tokens * 2
	streamCharCapMultiplier       = 4           // content char cap = max_tokens * 4 — safety net for runaway streams where token counting underestimates output. 2730 max_tokens → 10920 chars. Only fires after token-budget termination should have.
	maxHotInjectionChars          = 2000        // character cap for hot memory injection
)

var (
	toolCallInContent = regexp.MustCompile(`(?si)<tool_call>.*?</tool_call>`)
	extractionRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?s)\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"](.*?)['"]\s*\}?\s*\]?`),
		regexp.MustCompile(`(?s)\[?\s*\{\s*['"]type['"]\s*:\s*['"]text['"]\s*,\s*['"]text['"]\s*:\s*['"]([^'"]*)`),
	}

	// providerConstraints maps provider types to output constraint implementations.
	// Only local providers use GBNF grammar for tool call argument enforcement.
	// OpenAI-compatible endpoints (including local llama.cpp) cannot combine
	// grammar with tools in the same request — llama.cpp returns 400.
	providerConstraints = map[string]proxy.RequestConstraint{
		"local": &proxy.GBNFConstraint{},
	}
)

func (a *Agent) buildChatRequest(
	prepared []proxy.Message,
	llmTools []proxy.Tool,
) proxy.ChatRequest {
	req := proxy.ChatRequest{
		Messages:  prepared,
		Tools:     llmTools,
		MaxTokens: a.config.MaxTokens,
	}
	if a.config.UseNativeTools && len(llmTools) > 0 {
		req.ToolChoice = proxy.ToolChoiceRequired
	}
	if a.config.Temperature > 0 {
		req.Temperature = a.config.Temperature
	}
	if a.config.ReasoningBudget > 0 {
		req.SetReasoningBudget(a.config.ReasoningBudget)
	}
	// Apply provider-specific output constraint when native tools are active.
	// Local providers get GBNF grammar to prevent invalid JSON in tool call
	// arguments at the token generation level.  Cloud providers are skipped —
	// their native tool API already returns structured, valid JSON.
	if a.config.UseNativeTools && len(llmTools) > 0 {
		if constraint, ok := providerConstraints[a.config.ProviderType]; ok {
			if constraint.Apply(&req, llmTools) {
				a.deps.Logger.Debug("GBNF grammar applied for tool call constraint",
					"provider", a.config.ProviderType,
					"tools", len(llmTools))
			} else {
				a.deps.Logger.Warn("GBNF grammar could not be built (empty grammar)")
			}
		} else {
			a.deps.Logger.Debug("no output constraint for provider type",
				"provider", a.config.ProviderType)
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

	// Hot memory (<memory> block) is injected right before the last user
	// message for KV cache stability.  See docs/audits/memory-injection-investigation.md.
	if a.config.EnableHotMemory {
		prepared = a.injectActiveMemory(prepared, history)
	}

	var prefill string
	if a.shouldPrefill() {
		prefill = prompts.AutomationPrefline
		prepared = append(prepared, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: prefill,
		})
	}
	return prepared, prefill
}

// injectActiveMemory fetches all hot (mode:"always") entries via SearchHot
// and injects them as a <memory> system message before the last user message.
// Runs ONCE per session (first turn only).  The hot tag query replaces the old
// FTS5 search + separate user_profile fetch.  See docs/audits/memory-injection-investigation.md
// for the rationale and alternatives tried.
func (a *Agent) injectActiveMemory(prepared []proxy.Message, history []proxy.Message) []proxy.Message {
	if a.deps.MemoryStore == nil {
		return prepared
	}

	if a.memoryInjected {
		return prepared
	}
	a.memoryInjected = true

	ctx := context.Background()
	entries, err := a.deps.MemoryStore.SearchHot(ctx, a.config.WorkspaceID)
	if err != nil || len(entries) == 0 {
		return prepared
	}

	content := buildHotInjection(entries)
	if content == "" {
		return prepared
	}

	msg := proxy.Message{
		Role:    proxy.SystemRole,
		Content: "<memory>\n" + content + "\n</memory>",
	}

	// Insert right before the last user message for KV cache stability.
	insertIdx := len(prepared) - 1
	if insertIdx < 0 {
		insertIdx = 0
	}
	result := make([]proxy.Message, 0, len(prepared)+1)
	result = append(result, prepared[:insertIdx]...)
	result = append(result, msg)
	result = append(result, prepared[insertIdx:]...)
	return result
}

// buildHotInjection formats hot memory entries as "- Title: Content\n" lines.
// Truncates at maxHotInjectionChars on entry boundaries — never splits a fact.
func buildHotInjection(entries []memory.MemoryEntry) string {
	var b strings.Builder
	for _, e := range entries {
		line := fmt.Sprintf("- %s: %s\n", e.Title, e.Content)
		if b.Len()+len(line) > maxHotInjectionChars {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func (a *Agent) doPreflightCheck(
	ctx context.Context,
	history []proxy.Message,
	req *proxy.ChatRequest,
) (txnID string, err error) {
	if a.deps.Orchestrator == nil || a.deps.Orchestrator.Budget == nil {
		return "", nil
	}
	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
	}
	preflight, pfErr := a.deps.Orchestrator.Budget.PreFlightCheck(ctx, a.config.WorkspaceID,
		orchestrator.PreFlightRequest{
			ModelName:       a.config.ModelName,
			ProviderType:    a.config.ProviderType,
			ContextChars:    totalChars,
			MaxTokens:       a.config.MaxTokens,
			ReasoningBudget: a.config.ReasoningBudget,
			ICUWeight:       a.config.ICUWeight,
		})
	if pfErr != nil {
		return "", fmt.Errorf("budget error: %w", pfErr)
	}
	if !preflight.Allowed {
		return "", fmt.Errorf("budget exceeded: %s", preflight.Reason)
	}
	if preflight.SqueezeFactor < 1.0 {
		a.config.MaxTokens = preflight.AdjustedMaxTokens
		a.config.ReasoningBudget = preflight.AdjustedReasoning
		req.MaxTokens = a.config.MaxTokens
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
		Messages:  prepared,
		Tools:     nil,
		MaxTokens: a.config.MaxTokens,
	}
	if a.config.Temperature > 0 {
		req.Temperature = a.config.Temperature
	}
	return a.deps.Client.Stream(ctx, req)
}

func (a *Agent) handleEmptyStream(
	ctx context.Context, history []proxy.Message,
	tools []proxy.Tool, llmTools []proxy.Tool,
) (proxy.Message, error) {
	history = injectRetryContext(history)
	if llmTools != nil {
		// Native tools produced an empty response.  For native-only models
		// (usePrefill=false) the XML retry never produces tool calls either,
		// so skip it and go straight to non-streaming + nag prompt.
		// For models with prefill (usePrefill=true, i.e. local XML-text models)
		// the XML retry may still produce <tool_call> blocks.
		if !a.config.UsePrefill {
			a.deps.Logger.Info("empty response with native tools, returning stuck signal for nag recovery")
			return proxy.Message{
				Role:             proxy.AssistantRole,
				ReasoningContent: "[stuck]",
			}, nil
		}

		a.deps.Logger.Info("empty response with native tools, retrying in XML mode")
		a.notifyLifecycle("fallback_started", map[string]any{
			"reason": "empty stream with native tools", "mode": "xml",
		})
		savedNative := a.config.UseNativeTools
		a.config.UseNativeTools = false
		msg, err := a.computeNextResponseStreamXML(ctx, history, tools)
		a.config.UseNativeTools = savedNative
		return msg, err
	}
	return a.computeNextResponseNonStreaming(ctx, history, tools)
}

// isUserCanceled returns true when err signals that the user explicitly
// stopped the agent.  Used by fallback chains to bail out instead of
// retrying — re-attempting after a cancel is wasted work that races with
// the agent's outer loop seeing ctx.Err() and exiting.
func isUserCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// computeNextResponse tries streaming first, with fallback to non-streaming
// or XML-mode streaming on failure.  streamErr reuse across the deferred refund
// closure and subsequent retries is intentional (shadow-free single var).
func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	llmTools := tools
	if !a.config.UseNativeTools {
		llmTools = nil
	}

	prepared, prefill := a.prepareMessagesForTurn(history, tools, llmTools)
	req := a.buildChatRequest(prepared, llmTools)

	txnID, pfErr := a.doPreflightCheck(ctx, history, &req)
	if pfErr != nil {
		return proxy.Message{}, pfErr
	}

	ch, streamErr := a.deps.Client.Stream(ctx, req)
	a.deps.Logger.Info("stream request sent", "model", a.config.ModelName,
		"max_tokens", a.config.MaxTokens, "tool_choice", req.ToolChoice,
		"temperature", req.Temperature)

	if a.deps.Orchestrator != nil && a.deps.Orchestrator.Budget != nil && txnID != "" {
		defer func() {
			if streamErr != nil {
				if refErr := a.deps.Orchestrator.Budget.Refund(ctx, txnID); refErr != nil {
					a.deps.Logger.Warn("ICU refund failed", "txn", txnID, "error", refErr)
				} else {
					a.deps.Logger.Warn("ICU refunded due to stream failure", "txn", txnID)
				}
			}
		}()
	}

	if streamErr != nil {
		// User cancel — bail out, do not fall back to a retry path.
		// Falling back to non-streaming would issue another LLM call
		// against a dead context; the outer loop's ctx.Err() check is
		// the right place to handle termination.
		if isUserCanceled(streamErr) {
			return proxy.Message{}, streamErr
		}
		if prefill != "" && isPrefillThinkingError(streamErr) {
			a.deps.Logger.Info("prefill rejected by server, retrying without prefill")
			ch, streamErr = a.handlePrefillRejection(ctx, history, tools)
			prefill = ""
			if isUserCanceled(streamErr) {
				return proxy.Message{}, streamErr
			}
		}
		if streamErr != nil {
			a.deps.Logger.Warn("streaming not supported, falling back to non-streaming")
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
	if ReasoningBudgetExceeded(reasons, a.config.ReasoningBudget) {
		a.deps.Logger.Warn("reasoning budget far exceeded — server may not be enforcing",
			"reasoning_budget", a.config.ReasoningBudget,
			"reasoning_len", reasons,
			"model", a.config.ModelName)
	}
	a.deps.Logger.Info("stream completed",
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
	if a.config.SkipStuckCheck || len(fullMsg.Content) > 0 || len(fullMsg.ToolCalls) > 0 {
		return false
	}

	// Models with reasoningBudget == 0 get no server-side thinking enforcement.
	// If they produce reasoning content, catch them at maxTokens / divisor chars
	// rather than waiting for the full threshold.  Divisor=1 (maxTokens) avoids
	// false positives on models like Gemma 4 that output legitimate <think> blocks
	// before producing content/tool calls (~1371 chars observed).
	if a.config.ReasoningBudget == 0 && len(fullMsg.ReasoningContent) > a.config.MaxTokens/stuckNonReasoningDivisor {
		return true
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

	streamStartTime := time.Now()

	streamDone := make(chan struct{})
	defer close(streamDone)

	var streamContentLen, streamReasoningLen, streamToolCalls atomic.Int64

	go func() {
		ticker := time.NewTicker(streamHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				contentLen := int(streamContentLen.Load())
				reasoningLen := int(streamReasoningLen.Load())
				a.deps.Logger.Info("stream still generating", "content_len", contentLen, "reasoning_len", reasoningLen)
			case <-ctx.Done():
				return
			case <-streamDone:
				return
			}
		}
	}()

	logStreamEnd := func(reason string) {
		a.deps.Logger.Warn("stream ended",
			"end_reason", reason,
			"content_chars", len(fullMsg.Content),
			"reasoning_chars", len(fullMsg.ReasoningContent),
			"tool_calls", len(fullMsg.ToolCalls),
			"elapsed", time.Since(streamStartTime).Seconds())
	}

	for {
		select {
		case <-ctx.Done():
			logStreamEnd("context_canceled")
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

				if a.deps.Orchestrator != nil && a.deps.Orchestrator.Interceptor != nil {
					result := a.deps.Orchestrator.Interceptor.InterceptChunk(orchestrator.StreamChunk{
						Content:          chunkContent,
						ReasoningContent: reasoningChunk,
						ProviderType:     a.config.ProviderType,
					})
					tokUsed += result.TokensUsed
					reasonUsed += result.ReasoningUsed

				term := a.deps.Orchestrator.Interceptor.InterceptChunkWithBudget(ctx,
					orchestrator.StreamChunk{},
					tokUsed, reasonUsed, a.config.MaxTokens, a.config.ReasoningBudget,
				)
				if term.ShouldTerminate {
					// Upstream servers don't always enforce max_tokens, so
					// the client must end the stream when the interceptor
					// signals budget exceeded. Returning nil hands the
					// partial turn to the agent loop for evaluation.
				if !budgetWarned {
					budgetWarned = true
					a.deps.Logger.Warn("token budget exceeded, terminating stream",
						"tokens_used", tokUsed, "reasoning_used", reasonUsed,
						"token_budget", a.config.MaxTokens, "reasoning_budget", a.config.ReasoningBudget)
				}
				logStreamEnd("budget_exceeded")
				return nil
			}
			}

			if a.tryExtractToolCallFromReasoning(fullMsg) {
				logStreamEnd("extracted_tool_from_reasoning")
				return nil
			}

			if a.checkStreamStuck(fullMsg) {
				a.deps.Logger.Warn("reasoning stuck detected, aborting stream early to trigger fallback", "reasoning_chars", len(fullMsg.ReasoningContent), "stuck_threshold", a.stuckThreshold())
				a.notifyLifecycle("stuck_detected", map[string]any{
					"reasoning_chars": len(fullMsg.ReasoningContent),
				})
				logStreamEnd("stuck_detected")
				return nil
			}

			if chunkContent != "" {
				fullMsg.Content += chunkContent
			}
			if reasoningChunk != "" {
				fullMsg.ReasoningContent += reasoningChunk
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
			streamContentLen.Store(int64(len(fullMsg.Content)))
			streamReasoningLen.Store(int64(len(fullMsg.ReasoningContent)))
			streamToolCalls.Store(int64(len(fullMsg.ToolCalls)))

			if a.exceedsContentCharCap(fullMsg) {
				logStreamEnd("char_cap")
				a.deps.Logger.Warn("content char cap reached, terminating stream",
					"content_chars", len(fullMsg.Content),
					"cap", a.config.MaxTokens*streamCharCapMultiplier)
				return nil
			}

			if a.config.UseNativeTools && len(fullMsg.ToolCalls) == 0 && len(fullMsg.Content) > a.config.MaxTokens {
				a.deps.Logger.Warn("content exceeded max_tokens chars with no tool calls, terminating stream",
					"content_chars", len(fullMsg.Content),
					"cap", a.config.MaxTokens)
				return nil
			}

			if reasoningChunk != "" {
				a.notify(EventReasoning, fullMsg.ReasoningContent)
			}
			if chunkContent != "" {
				displayContent, _ := FilterStreamingMarkup(fullMsg.Content)
				if displayContent != "" {
					a.notify(EventToolStream, displayContent)
				}
			}
			}
		}
	}
}

func (a *Agent) retryWithoutTools(ctx context.Context, history []proxy.Message) (*proxy.ChatResponse, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentRetryTimeout)
	defer cancel()
	return a.deps.Client.Chat(chatCtx, proxy.ChatRequest{Messages: history, MaxTokens: a.config.MaxTokens})
}

func (a *Agent) computeNextResponseNonStreaming(ctx context.Context, history []proxy.Message, tools []proxy.Tool) (proxy.Message, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()

	llmTools := tools
	if !a.config.UseNativeTools {
		llmTools = nil
	}

	preparedHistory, prefill := a.prepareMessagesForTurn(history, tools, llmTools)

	req := a.buildChatRequest(preparedHistory, llmTools)

	if rawReq, err := json.Marshal(req); err == nil {
		a.deps.Logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	}

	a.deps.Logger.Info("non-stream request sent", "model", a.config.ModelName, "max_tokens", a.config.MaxTokens, "tool_choice", req.ToolChoice, "temperature", req.Temperature, "reasoning_budget", req.ReasoningBudget)

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

	resp, err := a.deps.Client.Chat(chatCtx, req)
	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.deps.Logger.Info("prefill rejected by server (thinking mode), retrying without prefill in XML mode (non-stream)")
		a.prefillDisabled = true
		a.notifyPrefillDisabled()
		prefill = ""
		preparedHistory = a.prepareMessages(history)
		if len(tools) > 0 {
			preparedHistory = a.injectToolInstructions(preparedHistory, tools)
		}
		req = proxy.ChatRequest{
			Messages:  preparedHistory,
			Tools:     nil,
			MaxTokens: a.config.MaxTokens,
		}
		resp, err = a.deps.Client.Chat(chatCtx, req)
	}
	if err != nil && isToolSupportError(err) {
		a.deps.Logger.Warn("model does not support tools, retrying without them", "error", err)
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
		Messages:  prepared,
		Tools:     nil,
		MaxTokens: a.config.MaxTokens,
	}
	if a.config.Temperature > 0 {
		req.Temperature = a.config.Temperature
	}

	a.deps.Logger.Info("xml stream retry sent", "model", a.config.ModelName, "max_tokens", a.config.MaxTokens, "prefill", prefill != "")

	ch, err := a.deps.Client.Stream(ctx, req)

	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.deps.Logger.Info("prefill rejected by server (thinking mode), retrying stream without prefill")
		a.prefillDisabled = true
		a.notifyPrefillDisabled()
		prefill = ""
		prepared, _ = a.prepareMessagesForTurn(history, tools, nil)
		req = proxy.ChatRequest{
			Messages:  prepared,
			Tools:     nil,
			MaxTokens: a.config.MaxTokens,
		}
		if a.config.Temperature > 0 {
			req.Temperature = a.config.Temperature
		}
		ch, err = a.deps.Client.Stream(ctx, req)
	}

	if err != nil {
		if isUserCanceled(err) {
			return proxy.Message{}, err
		}
		a.deps.Logger.Warn("xml stream retry failed, falling back to non-streaming", "error", err)
		return a.computeNextResponseNonStreaming(ctx, history, tools)
	}

	a.config.SkipStuckCheck = true
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	streamErr := a.processStream(ctx, ch, &fullMsg)
	a.config.SkipStuckCheck = false

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

	a.deps.Logger.Info("xml stream retry completed", "content_len", len(fullMsg.Content), "reasoning_len", len(fullMsg.ReasoningContent), "tool_calls", len(fullMsg.ToolCalls))
	return fullMsg, nil
}

func (a *Agent) shouldPrefill() bool {
	return a.config.UsePrefill && !a.prefillDisabled && !a.config.UseNativeTools
}

func (a *Agent) stuckThreshold() int {
	threshold := a.config.MaxTokens * stuckThresholdMultiplier
	if threshold < MinReasoningStuckThreshold {
		return MinReasoningStuckThreshold
	}
	return threshold
}

// exceedsContentCharCap is a safety net for runaway streams where the
// interceptor's token count underestimates output (e.g. provider returns
// content in a way the token counter mis-parses).  Caps the raw content
// character length at maxTokens * streamCharCapMultiplier.  Only fires after
// the token-budget termination should have; if it fires, the token
// counter has a bug we want to surface in logs.
func (a *Agent) exceedsContentCharCap(fullMsg *proxy.Message) bool {
	return a.config.MaxTokens > 0 && len(fullMsg.Content) > a.config.MaxTokens*streamCharCapMultiplier
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
