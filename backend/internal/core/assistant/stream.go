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

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/reasoning"
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
	streamReasoningBudgetDivisor = 3 // reasoning_budget = max_tokens / 3 — gives ~910 tokens for 2730 max_tokens, enough to review history and plan next tool call
	stuckNonReasoningDivisor     = 1 // early stuck threshold for non-reasoning models: maxTokens / divisor chars of pure reasoning triggers stuck. Divisor=1 gives threshold at maxTokens (e.g. 2048 for local models). Divisor=2 was too tight — Gemma 4 produces ~1371 chars of legitimate reasoning before outputting, causing false positives. Divisor=1 catches stuck 2x faster than the pre-change baseline (maxTokens*2) while giving reasoning-capable models room. See docs/audits/write-file-truncation-cycles.md.
	streamNotifyCoalesceInterval = 50 * time.Millisecond
	stuckThresholdMultiplier     = 2    // stuck threshold = max_tokens * 2
	streamCharCapMultiplier      = 4    // content char cap = max_tokens * 4 — safety net for runaway streams where token counting underestimates output. 2730 max_tokens → 10920 chars. Only fires after token-budget termination should have.
	maxHotInjectionChars         = 2000 // character cap for hot memory injection
	// emptyToolCallSpiralLimit: closed empty <tool_call></tool_call> blocks in pure-reasoning
	// streams trigger stuck early. Qwen 3.5 observed looping 100+ empty tags (~19s) before the
	// char threshold; abort at 3 closed empties so recovery (nag) starts in ~1s. Does not kill
	// the run — same stuck_detected → handleEmptyStream → nag path as char-threshold stuck.
	// Dangling open tags (still forming a real call) are not counted.
	emptyToolCallSpiralLimit = 3

	// Content-level repetition guard (Hermes Agent `repetition_guard` port).
	// A model in a degenerate loop can spend its output budget echoing one
	// fragment (observed: ~190 identical `</konjll>` closing tags, no tool
	// calls, no progress). Existing stuck/spiral detectors inspect reasoning or
	// parsed native tool calls; neither fires when visible content is dominated
	// by repeats. These constants mirror Hermes; see isRepetitionDominated.
	minRepetitionFragmentLen = 400 // below this the guard fails open
	repetitionWindow         = 60  // exact-repeat window length
	minRepetitionCount       = 5   // repeats of a window required to signal
	repetitionDominanceRatio = 0.5 // fraction of content a repeat must cover
)

// Heartbeat cadences are variables (not consts) so the stream/non-stream
// liveness tests can shorten them without waiting for the production cadence.
var (
	streamHeartbeatInterval    = 30 * time.Second // progress log during long streams
	nonStreamHeartbeatInterval = 15 * time.Second // fallback_waiting lifecycle event
	// streamMaxDuration bounds a single stream that is producing no native
	// tool calls and no natural completion, so a slow degenerate stream cannot
	// run ~50s+ unchecked (the pre-change per-turn timeout is 10 minutes).
	// Fires via the heartbeat tick; test-shortenable like the heartbeat.
	streamMaxDuration = 90 * time.Second
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
	a.applyRequestConfig(&req)
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

// applyRequestConfig applies per-model request defaults shared by every LLM
// call path — the turn loop (via buildChatRequest) and plan generation (via
// ExecutionPlanStrategy.Generate): temperature and the reasoning wire params.
// Single source of truth for both concerns.
func (a *Agent) applyRequestConfig(req *proxy.ChatRequest) {
	if a.config.Temperature > 0 {
		req.Temperature = a.config.Temperature
	}
	// Single source of truth for reasoning wire params. The resolver is chosen
	// by workload class + provider type; applyRequestConfig knows nothing about
	// per-provider wire details (Dependency Inversion).
	// Runtime guard: the final spec must pass Validate() before hitting the
	// wire. An invalid spec (e.g. ModeObject + EffortNone) would otherwise
	// reach the provider; on failure we emit no reasoning params at all.
	if err := a.config.ReasoningSpec.Validate(); err != nil {
		if a.deps.Logger != nil {
			a.deps.Logger.Error("reasoning spec invalid; omitting reasoning params", "error", err)
		}
	} else {
		resolver := reasoning.NewReasoningResolver(a.config.WorkloadClass, a.config.ProviderType, a.config.ReasoningBudget)
		resolver.Apply(req, a.config.ReasoningSpec)
	}
	if a.deps.Logger != nil {
		a.deps.Logger.Debug("reasoning resolver applied",
			"provider", a.config.ProviderType,
			"mode", int(a.config.ReasoningSpec.Mode),
			"budget", a.config.ReasoningSpec.Budget)
	}
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

	if a.memoryInjected() {
		return prepared
	}
	a.setMemoryInjected(true)

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
	a.setPrefillDisabled(true)
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
	tools []proxy.Tool, llmTools []proxy.Tool, toolChoice proxy.ToolChoice,
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
		msg, err := a.computeNextResponseStreamXML(ctx, history, tools, toolChoice)
		a.config.UseNativeTools = savedNative
		return msg, err
	}
	// Empty text-only turn after the recovery ladder already forced a
	// finalization attempt: do NOT issue another LLM request. Retrying via
	// non-stream would burn an upstream call that can surface as a 503 and mask
	// the ladder's intended graceful termination. Return the stuck signal so
	// handleNoToolCalls falls through to bestAvailableAnswer().
	if a.runS != nil && a.runS.finalizeAttempts >= 1 {
		a.deps.Logger.Info("empty finalization turn — returning stuck signal for terminal recovery",
			"finalizeAttempts", a.runS.finalizeAttempts)
		return proxy.Message{
			Role:             proxy.AssistantRole,
			ReasoningContent: "[stuck]",
		}, nil
	}
	return a.computeNextResponseNonStreaming(ctx, history, tools, toolChoice)
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
func (a *Agent) computeNextResponse(ctx context.Context, history []proxy.Message, tools []proxy.Tool, toolChoice proxy.ToolChoice) (proxy.Message, error) {
	llmTools := tools
	if !a.config.UseNativeTools {
		llmTools = nil
	}

	prepared, prefill := a.prepareMessagesForTurn(history, tools, llmTools)
	req := a.buildChatRequest(prepared, llmTools)
	if toolChoice != "" {
		req.ToolChoice = toolChoice
	}

	txnID, pfErr := a.doPreflightCheck(ctx, history, &req)
	if pfErr != nil {
		return proxy.Message{}, pfErr
	}

	// Signal the UI that the agent is now working (reasoning compute / pre-token
	// wait) before any response content arrives. Neutral status only.
	a.notifyAgentThinking()

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
			// Distinguish transport failures from genuine "streaming not
			// supported" rejections so the operator knows WHY the stream died:
			// an upstream connection drop (unexpected EOF / timeout / reset)
			// means the provider never responded, while an HTTP-level error
			// means the provider answered but rejected streaming. Both fall
			// back to non-streaming (same as before), but the log now says
			// which one it was.
			var terr *proxy.TransportError
			if errors.As(streamErr, &terr) {
				a.deps.Logger.Warn("upstream transport failure, falling back to non-streaming",
					"error_class", terr.Class, "elapsed", terr.Elapsed.Round(time.Second).String(), "error", streamErr)
			} else {
				a.deps.Logger.Warn("streaming not supported, falling back to non-streaming", "error", streamErr)
			}
			a.setMemoryInjected(false) // retry injection on the non-streaming path
			return a.computeNextResponseNonStreaming(ctx, history, tools, toolChoice)
		}
	}

	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if streamErr = a.processStream(ctx, ch, &fullMsg, hasToolResult(history), len(tools) > 0); streamErr != nil {
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
		return a.handleEmptyStream(ctx, history, tools, llmTools, toolChoice)
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
//
// Also aborts on empty-tool_call spiral: N closed empty <tool_call></tool_call>
// blocks in reasoning with no content/native tools (see emptyToolCallSpiralLimit).
func (a *Agent) checkStreamStuck(fullMsg *proxy.Message) bool {
	if a.config.SkipStuckCheck || len(fullMsg.Content) > 0 || len(fullMsg.ToolCalls) > 0 {
		return false
	}

	// Empty closed tool_call tags looping in pure reasoning (Qwen 3.5 etc).
	// Catch before char threshold so recovery starts in ~1s instead of ~19s.
	if countEmptyClosedToolCalls(fullMsg.ReasoningContent) >= emptyToolCallSpiralLimit {
		return true
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

// repetitionDominated reports whether the streamed visible content is
// dominated by verbatim repeated fragments while no native tool calls have
// been parsed. This is the content-level complement to checkStreamStuck: the
// latter only fires on reasoning-only streams, whereas a model in a
// degenerate loop can also write visible content (e.g. a malformed tool-call
// dialect echoed as ~190 closing tags) with no tool calls and no progress.
// Real tool calls are never discarded — the guard requires zero parsed calls.
func (a *Agent) repetitionDominated(fullMsg *proxy.Message) bool {
	return len(fullMsg.ToolCalls) == 0 && isRepetitionDominated(fullMsg.Content)
}

// countEmptyClosedToolCalls counts closed <tool_call>...</tool_call> blocks whose
// body is empty/whitespace. Dangling open tags (still forming) are not counted.
// Case-insensitive to match containsSubstantiveToolCall.
func countEmptyClosedToolCalls(s string) int {
	count := 0
	lower := strings.ToLower(s)
	for {
		idx := strings.Index(lower, "<tool_call>")
		if idx == -1 {
			return count
		}
		closeIdx := strings.Index(lower[idx:], "</tool_call>")
		if closeIdx == -1 {
			return count // dangling open — still forming, do not count
		}
		body := strings.TrimSpace(s[idx+len("<tool_call>") : idx+closeIdx])
		if body == "" {
			count++
		}
		lower = lower[idx+closeIdx+len("</tool_call>"):]
		s = s[idx+closeIdx+len("</tool_call>"):]
	}
}

// containsSubstantiveToolCall checks whether s contains at least one <tool_call>
// block with a non-empty body (after trimming whitespace).  An empty block
// (<tool_call></tool_call> or <tool_call>  \n  </tool_call>) means the model
// hasn't finished forming the call — the stream should keep going so it can
// fill it in.  Tag matching is case-insensitive to align with the (?si) regex.
func containsSubstantiveToolCall(s string) bool {
	lower := strings.ToLower(s)
	for {
		idx := strings.Index(lower, "<tool_call>")
		if idx == -1 {
			return false
		}
		closeIdx := strings.Index(lower[idx:], "</tool_call>")
		if closeIdx == -1 {
			return false // dangling open tag — still forming
		}
		body := strings.TrimSpace(s[idx+len("<tool_call>") : idx+closeIdx])
		if body != "" {
			return true // found a substantive call
		}
		// Empty block — skip past it and check for more
		lower = lower[idx+closeIdx+len("</tool_call>"):]
		s = s[idx+closeIdx+len("</tool_call>"):]
	}
}

// isRepetitionDominated reports whether text is dominated by verbatim
// repeated fragments. A fragment is "repetition-dominated" when a single
// 60+ char substring appears often enough that its occurrences cover at
// least half of the text — the signature of a model repetition loop
// (Hermes Agent `repetition_guard.is_repetition_dominated`, incident
// #86581). Returns false for short inputs (fail-open: never flags content
// the guard cannot confidently judge). Model- and provider-agnostic: it keys
// purely off the streamed bytes, so it is independent of grammar/tool format.
func isRepetitionDominated(text string) bool {
	n := len(text)
	if n < minRepetitionFragmentLen {
		return false
	}

	// Fast path: one normalized line duplicated often enough to cover half
	// the fragment (a repeated paragraph or sentence on its own line).
	if lineRepetitionDominated(text, n) {
		return true
	}

	// General path: fixed-size exact-repeat windows, sliding one char at a
	// time. Catches repetition loops that do not align to line boundaries
	// (e.g. a repeated closing tag on a single line). Integer math keeps this
	// exact: ratio/window = 0.5/60 = 1/120, so the required count is
	// ceil(n/120) = (n+119)/120.
	needed := max(minRepetitionCount, (n+repetitionWindow-1)/(repetitionWindow*2))
	counts := make(map[string]int, n-repetitionWindow+1)
	for i := 0; i <= n-repetitionWindow; i++ {
		key := text[i : i+repetitionWindow]
		counts[key]++
		if counts[key] >= needed {
			return true
		}
	}
	return false
}

func lineRepetitionDominated(text string, n int) bool {
	counts := make(map[string]int)
	for _, line := range strings.Split(text, "\n") {
		norm := strings.TrimSpace(line)
		if norm == "" {
			continue
		}
		counts[norm]++
	}
	half := (n + 1) / 2 // ceil(n*0.5)
	for line, c := range counts {
		if c >= minRepetitionCount && c*len(line) >= half {
			return true
		}
	}
	return false
}

// tryExtractToolCallFromReasoning handles models (e.g. Qwen 3.5) that emit
// <tool_call> blocks inside reasoning content, where they're invisible
// to the native-tool parser but still valid XML.  Extracts tool calls
// directly into fullMsg.ToolCalls without copying reasoning text into
// Content — reasoning text belongs to thinking, not visible output.
// The llama.cpp server already separates reasoning_content from content
// at the wire level; this function bridges the gap when the model writes
// tool calls as text inside reasoning instead of using native deltas.
func (a *Agent) tryExtractToolCallFromReasoning(fullMsg *proxy.Message) bool {
	if len(fullMsg.ReasoningContent) == 0 {
		return false
	}
	// If the model already wrote visible content, the tool call (if any)
	// came through the native path or is already in Content.  Do not
	// inject reasoning text on top of real user-facing output.
	if len(fullMsg.Content) > 0 {
		return false
	}
	if !toolCallInContent.MatchString(fullMsg.ReasoningContent) {
		return false
	}
	cleaned := cleanReasoningContent(fullMsg.ReasoningContent)
	if cleaned == "" {
		return false
	}
	// Only extract if the tool call block contains a non-empty body.
	// An empty <tool_call></tool_call> means the model hasn't finished
	// forming the call — keep streaming so it can fill it in.
	if !containsSubstantiveToolCall(fullMsg.ReasoningContent) {
		return false
	}
	_, calls, parseErr := proxy.ParseContentToolCalls(cleaned)
	if parseErr != nil || len(calls) == 0 {
		return false
	}
	fullMsg.ToolCalls = append(fullMsg.ToolCalls, calls...)
	return true
}

// resolveStreamChunk resolves one stream chunk's content/reasoning fields into
// a Message using the Delta→Message fallback (Delta wins; Message only when the
// Delta field is empty). It is the exact field semantics shared by
// processStream and plan-generation streaming — a single source of truth, so a
// new provider reasoning field is learned by every consumer in one place.
func resolveStreamChunk(choice proxy.Choice) proxy.Message {
	msg := choice.Delta
	if msg.Content == "" {
		msg.Content = choice.Message.Content
	}
	if msg.ReasoningContent == "" {
		msg.ReasoningContent = choice.Message.ReasoningContent
	}
	// Openrouter-style opaque/structured reasoning (ReasoningObject).
	if msg.Reasoning == "" {
		msg.Reasoning = choice.Message.Reasoning
	}
	if len(msg.ReasoningDetails) == 0 {
		msg.ReasoningDetails = choice.Message.ReasoningDetails
	}
	return msg
}

// processStream consumes an LLM stream, accumulating content/reasoning/tool
// calls into fullMsg and terminating the stream when a budget or safety cap is
// reached. The relaxation flags let the caller suppress the no-tool content cap
// for turns that are plausibly a legitimate final answer:
//   - priorToolResult: history already contains a ToolRole (real work was done)
//   - toolsAvailable:  at least one tool is configured for this turn
//
// When both are false, a long tool-free answer is most likely the runaway
// joke-loop, so the cap stays armed. See §2.1 of the automation renderer plan.
func (a *Agent) processStream(ctx context.Context, ch <-chan *proxy.ChatResponse, fullMsg *proxy.Message, priorToolResult, toolsAvailable bool) error {
	var tokUsed, reasonUsed int
	var budgetWarned bool
	var relaxedCapWarned bool

	streamStartTime := time.Now()

	var pendingReasoning, pendingContent string
	var lastEmittedReasoning, lastEmittedContent string
	var lastNotifyAt time.Time
	flushPendingNotify := func() {
		if pendingReasoning != "" && pendingReasoning != lastEmittedReasoning {
			a.notify(EventReasoning, pendingReasoning)
			lastEmittedReasoning = pendingReasoning
			pendingReasoning = ""
		}
		if pendingContent != "" && pendingContent != lastEmittedContent {
			a.notify(EventToolStream, pendingContent)
			lastEmittedContent = pendingContent
			pendingContent = ""
		}
		lastNotifyAt = time.Now()
	}
	defer flushPendingNotify()

	var streamContentLen, streamReasoningLen atomic.Int64

	// Liveness heartbeat: emits still_thinking only while the stream is silent
	// (no content/reasoning advanced since the last tick) so the UI never shows
	// a dead bubble during a long provider TTFT or silent-stall period.
	hb := core.NewHeartbeat()
	hb.Start(ctx, streamHeartbeatInterval)
	defer hb.Stop()
	var lastTickContent, lastTickReasoning int64

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
		case <-hb.C:
			contentLen := streamContentLen.Load()
			reasoningLen := streamReasoningLen.Load()
			a.deps.Logger.Info("stream still generating", "content_len", contentLen, "reasoning_len", reasoningLen)
			if contentLen == lastTickContent && reasoningLen == lastTickReasoning {
				// Silent-stall gate: only signal liveness when nothing advanced
				// since the last tick, so active streaming stays quiet.
				a.notifyLifecycle(PhaseStillThinking, map[string]any{
					"elapsed": time.Since(streamStartTime).Round(time.Second).String(),
				})
			}
			lastTickContent, lastTickReasoning = contentLen, reasoningLen
			// Per-stream duration cap: a stream producing no native tool calls
			// and no natural completion beyond the cap is degenerate (e.g. a
			// slow, varied garbage loop the repetition guard does not catch).
			// Bound it well under the per-turn timeout so the recovery ladder
			// can run instead of streaming unchecked for minutes. Unlike the
			// repetition guard, the accumulated content is preserved (it may be
			// a genuine slow report) — mirroring the char-cap termination so
			// handleTextTurn can complete or salvage it.
			if len(fullMsg.ToolCalls) == 0 && time.Since(streamStartTime) > streamMaxDuration {
				a.deps.Logger.Warn("stream exceeded max duration with no tool calls, terminating stream",
					"elapsed", time.Since(streamStartTime).Round(time.Second).String(),
					"content_chars", len(fullMsg.Content),
					"reasoning_chars", len(fullMsg.ReasoningContent))
				logStreamEnd("stream_timeout")
				return nil
			}
		case resp, ok := <-ch:
			if !ok {
				return nil
			}
			if len(resp.Choices) > 0 {
				choice := resp.Choices[0]
				chunk := resolveStreamChunk(choice)
				chunkContent := chunk.Content
				reasoningChunk := chunk.ReasoningContent
				reasoningStr := chunk.Reasoning
				reasoningDetails := chunk.ReasoningDetails

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
					emptyCalls := countEmptyClosedToolCalls(fullMsg.ReasoningContent)
					a.deps.Logger.Warn("reasoning stuck detected, aborting stream early to trigger fallback",
						"reasoning_chars", len(fullMsg.ReasoningContent),
						"stuck_threshold", a.stuckThreshold(),
						"empty_tool_calls", emptyCalls)
					a.notifyLifecycle("stuck_detected", map[string]any{
						"reasoning_chars":  len(fullMsg.ReasoningContent),
						"empty_tool_calls": emptyCalls,
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
				if reasoningStr != "" {
					fullMsg.Reasoning += reasoningStr
				}
				if len(reasoningDetails) > 0 {
					fullMsg.ReasoningDetails = append(fullMsg.ReasoningDetails, reasoningDetails...)
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

				if a.exceedsContentCharCap(fullMsg) {
					logStreamEnd("char_cap")
					a.deps.Logger.Warn("content char cap reached, terminating stream",
						"content_chars", len(fullMsg.Content),
						"cap", a.config.MaxTokens*streamCharCapMultiplier)
					return nil
				}

				// Content-level repetition guard: catch a degenerate loop that
				// writes visible content (e.g. a malformed tool-call dialect
				// echoed as repeated closing tags) with no tool calls and no
				// progress. Runs after content accumulation so it sees the
				// dominated text; never fires when real tool calls are parsed.
				if a.repetitionDominated(fullMsg) {
					a.deps.Logger.Warn("content repetition detected, aborting stream early to trigger fallback",
						"content_chars", len(fullMsg.Content))
					a.abortStreamAsStuck("repetition_detected", fullMsg, logStreamEnd, map[string]any{
						"reason":        "content_repetition",
						"content_chars": len(fullMsg.Content),
					})
					return nil
				}

				if a.config.UseNativeTools && len(fullMsg.ToolCalls) == 0 && len(fullMsg.Content) > a.config.MaxTokens {
					// Relax the cap for turns that are plausibly a genuine final
					// answer: real work already happened (prior tool result) or no
					// tools are configured so a pure-text answer is expected. Let the
					// stream run to its natural stop so checkTaskCompletion receives
					// the full answer intact. Keep the cap armed otherwise — a
					// tool-free turn with no prior work is the runaway joke-loop.
					if !priorToolResult && toolsAvailable {
						if !relaxedCapWarned {
							relaxedCapWarned = true
							a.deps.Logger.Warn("content exceeded max_tokens chars with no tool calls, terminating stream",
								"content_chars", len(fullMsg.Content),
								"cap", a.config.MaxTokens)
						}
						return nil
					}
					if !relaxedCapWarned {
						relaxedCapWarned = true
						a.deps.Logger.Warn("content exceeded max_tokens chars with no tool calls, allowing stream to continue (legitimate final answer candidate)",
							"content_chars", len(fullMsg.Content),
							"cap", a.config.MaxTokens,
							"prior_tool_result", priorToolResult,
							"tools_available", toolsAvailable)
					}
				}

				if reasoningChunk != "" || reasoningStr != "" || len(reasoningDetails) > 0 {
					if disp := fullMsg.ExtractReasoning(); disp != "" {
						pendingReasoning = disp
					}
				}
				if chunkContent != "" {
					displayContent, _ := FilterStreamingMarkup(fullMsg.Content)
					if displayContent != "" {
						pendingContent = displayContent
					}
				}

				if time.Since(lastNotifyAt) >= streamNotifyCoalesceInterval {
					flushPendingNotify()
				}
			}
		}
	}
}

// abortStreamAsStuck terminates a degenerate stream into the existing stuck
// recovery ladder. The accumulated content is garbage (a repetition loop), so
// it is discarded and the turn is signalled as [stuck]: computeNextResponse
// then routes through handleEmptyStream → handleNoToolCalls → the
// progressive-sieve nag recovery, exactly like char-threshold stuck. The
// repetition guard only invokes this with zero parsed tool calls, so nothing
// legitimate is discarded.
func (a *Agent) abortStreamAsStuck(reason string, fullMsg *proxy.Message, logStreamEnd func(string), extra map[string]any) {
	a.notifyLifecycle("stuck_detected", extra)
	logStreamEnd(reason)
	fullMsg.Content = ""
	fullMsg.ReasoningContent = "[stuck]"
}

func (a *Agent) retryWithoutTools(ctx context.Context, history []proxy.Message) (*proxy.ChatResponse, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentRetryTimeout)
	defer cancel()
	return a.deps.Client.Chat(chatCtx, proxy.ChatRequest{Messages: history, MaxTokens: a.config.MaxTokens})
}

func (a *Agent) computeNextResponseNonStreaming(ctx context.Context, history []proxy.Message, tools []proxy.Tool, toolChoice proxy.ToolChoice) (proxy.Message, error) {
	chatCtx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()

	llmTools := tools
	if !a.config.UseNativeTools {
		llmTools = nil
	}

	preparedHistory, prefill := a.prepareMessagesForTurn(history, tools, llmTools)

	req := a.buildChatRequest(preparedHistory, llmTools)
	if toolChoice != "" {
		req.ToolChoice = toolChoice
	}

	if rawReq, err := json.Marshal(req); err == nil {
		a.deps.Logger.Debug("Outgoing LLM Non-Stream Request", "payload", string(rawReq))
	}

	a.deps.Logger.Info("non-stream request sent", "model", a.config.ModelName, "max_tokens", a.config.MaxTokens, "tool_choice", req.ToolChoice, "temperature", req.Temperature, "reasoning_budget", req.ReasoningBudget)

	// Neutral "working" status before the (single) response arrives.
	a.notifyAgentThinking()

	hb := core.NewHeartbeat()
	hb.Start(ctx, nonStreamHeartbeatInterval)
	defer hb.Stop()
	heartbeatStart := time.Now()

	// wait runs a single blocking LLM call in a goroutine and consumes the
	// heartbeat ticker while waiting, so the UI keeps receiving fallback_waiting
	// liveness during a slow provider response. Every retry path reuses it. The
	// goroutine terminates when fn returns, which is bounded by the ctx/chatCtx
	// timeouts it observes (no leak on ctx.Done).
	wait := func(fn func() (*proxy.ChatResponse, error)) (*proxy.ChatResponse, error) {
		type res struct {
			resp *proxy.ChatResponse
			err  error
		}
		result := make(chan res, 1)
		go func() {
			resp, err := fn()
			result <- res{resp, err}
		}()
		for {
			select {
			case <-hb.C:
				elapsed := time.Since(heartbeatStart).Round(time.Second)
				a.notifyLifecycle("fallback_waiting", map[string]any{
					"elapsed": elapsed.String(),
				})
			case <-ctx.Done():
				return nil, ctx.Err()
			case r := <-result:
				return r.resp, r.err
			}
		}
	}

	resp, err := wait(func() (*proxy.ChatResponse, error) { return a.deps.Client.Chat(chatCtx, req) })
	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.deps.Logger.Info("prefill rejected by server (thinking mode), retrying without prefill in XML mode (non-stream)")
		a.setPrefillDisabled(true)
		a.notifyPrefillDisabled()
		prefill = ""
		preparedHistory = a.prepareMessages(history)
		if len(tools) > 0 {
			preparedHistory = a.injectToolInstructions(preparedHistory, tools)
		}
		req = proxy.ChatRequest{
			Messages:   preparedHistory,
			Tools:      nil,
			ToolChoice: toolChoice,
			MaxTokens:  a.config.MaxTokens,
		}
		resp, err = wait(func() (*proxy.ChatResponse, error) { return a.deps.Client.Chat(chatCtx, req) })
	}
	if err != nil && isToolSupportError(err) {
		a.deps.Logger.Warn("model does not support tools, retrying without them", "error", err)
		a.notifyFallbackWarning(err)
		resp, err = wait(func() (*proxy.ChatResponse, error) { return a.retryWithoutTools(ctx, history) })
	}
	if err != nil && isUnsupportedParameterError(err) {
		a.deps.Logger.Warn("provider rejected unsupported parameter, retrying without reasoning params", "error", err)
		proxy.ClearReasoningParams(&req)
		resp, err = wait(func() (*proxy.ChatResponse, error) { return a.deps.Client.Chat(chatCtx, req) })
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

func (a *Agent) computeNextResponseStreamXML(ctx context.Context, history []proxy.Message, tools []proxy.Tool, toolChoice proxy.ToolChoice) (proxy.Message, error) {
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

	// Neutral "working" status before the stream begins.
	a.notifyAgentThinking()

	ch, err := a.deps.Client.Stream(ctx, req)

	if err != nil && prefill != "" && isPrefillThinkingError(err) {
		a.deps.Logger.Info("prefill rejected by server (thinking mode), retrying stream without prefill")
		a.setPrefillDisabled(true)
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
		return a.computeNextResponseNonStreaming(ctx, history, tools, toolChoice)
	}

	a.config.SkipStuckCheck = true
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	streamErr := a.processStream(ctx, ch, &fullMsg, hasToolResult(history), len(tools) > 0)
	a.config.SkipStuckCheck = false

	if streamErr != nil {
		return proxy.Message{}, streamErr
	}

	if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 {
		saved := a.prefillDisabled()
		a.setPrefillDisabled(true)
		msg, err := a.computeNextResponseNonStreaming(ctx, history, tools, toolChoice)
		a.setPrefillDisabled(saved)
		return msg, err
	}

	if prefill != "" {
		fullMsg.Content = prefill + fullMsg.Content
	}

	a.deps.Logger.Info("xml stream retry completed", "content_len", len(fullMsg.Content), "reasoning_len", len(fullMsg.ReasoningContent), "tool_calls", len(fullMsg.ToolCalls))
	return fullMsg, nil
}

func (a *Agent) shouldPrefill() bool {
	return a.config.UsePrefill && !a.prefillDisabled() && !a.config.UseNativeTools
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

var thinkTagsRe = regexp.MustCompile(`</?think>`)

func cleanReasoningContent(s string) string {
	return strings.TrimSpace(thinkTagsRe.ReplaceAllString(s, ""))
}

func FilterStreamingMarkup(content string) (displayContent string, hasToolCall bool) {
	cutoffPatterns := []string{
		"<function-name>", "</function-name>", "<args-json-object>", "</args-json-object>",
		"<tools>", "functions.",
		"<|tool_call", "<tool_call",
		"[TOOL_CALLS]", "[ARGS]",
		"```json",
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
