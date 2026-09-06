// session.go — runSession struct (encapsulates one Execute call), turn
// execution, no-tool-call heuristics, content tool call parsing, termination
// detection, and the main agentic loop body.
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"llm-proxy/internal/core/assistant/failures"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

const (
	sessionMaxSieveRetries        = 3    // consecutive context-overflow errors before giving up
	sessionMinHistoryForSieve     = 2    // fewer messages and the sieve can't operate
	sessionModelCompatNotifyAfter = 5    // consecutive parse errors before suggesting model swap
	sessionWriteTrimThreshold     = 1000 // chars above which write_file content is replaced with a stub
	sessionPreviewMaxLen          = 500  // chars for parse-error content preview in log
	sessionTruncationFeedbackLen  = 400  // chars above which truncated content gets a guidance message
	sessionParseErrorEscalation   = 2    // same-error-kind streak before escalating feedback
	sessionMinMonologueLen        = 10   // chars below which repetition check is skipped
	sessionMaxSyntaxParseRetries  = 3    // consecutive server-side JSON syntax errors before giving up
	// Max control-plane messages to skip when resolving conversation adjacency.
	// Normal recovery inserts 1–3; this is a safety cap only.
	sessionControlWalkCap = 8

	MinAnswerContentLength = 20  // chars threshold for meaningful assistant content
	MemoryFlushRatio       = 0.7 // context budget fraction that triggers memory flush

	// postToolNudgeMax is the number of times handleNoToolCalls will re-nag the
	// model immediately after a tool result before forcing a tools-disabled
	// finalization turn. Re-armed on every successful tool turn.
	postToolNudgeMax = 2

	// lengthContinuationMax bounds how many times a finish_reason="length"
	// final answer is nudged to continue where it left off before the partial
	// is accepted as-is. Mirrors Hermes's length-continuation (which allows 4);
	// we keep it tighter to bound total turns while still repairing the common
	// single-cutoff case (a report truncated once by the output cap).
	lengthContinuationMax = 2

	// maxStopGuardAttempts bounds the number of times a stop guard may nudge the
	// run past a natural-completion candidate before the guard must allow
	// finalization. Dedicated counter — never finalizeAttempts (owned by the
	// empty-turn recovery ladder). A guard must never nag perpetually.
	maxStopGuardAttempts = 2
)

// runSession encapsulates the mutable state of one Agent.Execute call.
// Fields replace the old pointer-to-primitive pattern (was *int, *bool, *string).
type runSession struct {
	agent *Agent
	ctx   context.Context

	history         []proxy.Message
	steps           int
	sieveStreak     int
	starvationCount int
	warnedAdvisory  bool

	parseErrorStreak    int
	lastParseErrorKind  string
	totalErrorStreak    int
	modelCompatNotified bool
	// postToolNudgeCount re-arms on every successful tool turn (reset in
	// resetParseErrorState), so a model that emits empty responses after running
	// tools is nudged repeatedly until it produces a report. hardCapTriggered
	// (independent) gates the forced completion at MaxSteps*2.
	postToolNudgeCount int
	hardCapTriggered   bool
	finalizeAttempts   int  // how many deterministic tools-disabled finalization turns fired
	textOnlyNextTurn   bool // next executeTurn runs with tools disabled (ToolChoiceNone)

	// Guardrail-blocked tool loop guard: consecutive tool calls denied by the
	// guardrail engine (policy block or user denial). A model that keeps
	// retrying a blocked tool with NEW arguments is invisible to the
	// args-keyed spiral detector, so the streak is tracked on the result text
	// (appendToolResult); past guardrailBlockStreakLimit the next turn injects
	// a targeted nag and raises the sampling temperature to break the rut.
	guardrailBlockStreak   int
	guardrailBlockedTool   string
	guardrailNagSent       bool
	recoveryTempEscalation float64

	// lengthContinuationCount tracks finish_reason="length" continuation
	// nudges injected for a truncated final answer (bounded by
	// lengthContinuationMax). truncatedParts accumulates the partial report
	// fragments so the completed answer is the full stitched text, matching
	// Hermes's _join_truncated_parts behavior.
	lengthContinuationCount int
	truncatedParts          []string
	syntaxParseStreak       int // consecutive server-side tool-arg JSON syntax failures

	rd                   repetitionDetector
	memoryFlushSent      bool   // prevents repeated pre-sieve nudges across turns
	lastContentWithTools string // content saved from a turn that had both text and tool calls

	// Stop guards (evaluator-optimizer, Phase 3). nil for plain react — with no
	// guards maybeNudge always allows finalization (zero behavior change).
	// stopGuardAttempts is a dedicated bounded counter (cap maxStopGuardAttempts),
	// never finalizeAttempts, which handleNoToolCalls owns for its tools-disabled
	// finalization turn.
	stopGuards        []StopGuard
	stopGuardAttempts int

	prefillDisabled bool // runtime override to skip prefill on retry
	memoryInjected  bool // gates hot-memory injection to first turn only
}

func newRunSession(agent *Agent, ctx context.Context, history []proxy.Message) *runSession {
	return &runSession{
		agent:   agent,
		ctx:     ctx,
		history: append([]proxy.Message{}, history...),
	}
}

func (s *runSession) handleContextSizeError() error {
	s.sieveStreak++
	if s.sieveStreak >= sessionMaxSieveRetries {
		return fmt.Errorf("agent execution failed: model stuck in reasoning loop after %d sieve retries", s.sieveStreak)
	}
	// First sieve uses reactive (keep 2 + 6 messages),
	// subsequent attempts use aggressive (keep 2 + 3 messages)
	if s.sieveStreak == 1 {
		s.history = s.agent.applyReactiveSieve(s.history)
		s.history = append(s.history, proxy.Message{
			Role:    proxy.UserRole,
			Content: prompts.ReasoningStuckNag,
		})
	} else {
		if len(s.history) <= sessionMinHistoryForSieve {
			return fmt.Errorf("agent execution failed: cannot recover from reasoning loop")
		}
		s.history = s.agent.applyAggressiveSieve(s.history)
		s.history = append(s.history, proxy.Message{
			Role:    proxy.UserRole,
			Content: prompts.ReasoningStuckEscalatedNag,
		})
	}
	return nil
}

// handleToolCallParseError injects recovery feedback for server-side tool-arg
// JSON failures. Returns true when the session should stop retrying (capped
// syntax streak) — caller exits with best available answer or stall error.
func (s *runSession) handleToolCallParseError(err error) (giveUp bool) {
	switch {
	case failures.IsJSONSyntaxError(err):
		s.syntaxParseStreak++
		s.agent.deps.Logger.Warn("server-side tool call JSON parse error (syntax), sending JSON-escaped hint",
			"error", err, "streak", s.syntaxParseStreak)
		if s.syntaxParseStreak >= sessionMaxSyntaxParseRetries {
			return true
		}
		s.history = append(s.history, proxy.Message{
			Role:    proxy.UserRole,
			Content: prompts.AutomationJSONSyntaxPrompt,
		})
	default:
		s.agent.deps.Logger.Warn("server-side tool call JSON parse error, sending length feedback to model", "error", err)
		s.totalErrorStreak++
		if s.totalErrorStreak >= sessionModelCompatNotifyAfter && !s.modelCompatNotified {
			s.modelCompatNotified = true
			s.agent.notifyModelCompatWarning(s.agent.config.UseNativeTools)
		}
		s.history = append(s.history, proxy.Message{
			Role:    proxy.UserRole,
			Content: prompts.AutomationContentTooLongPrompt,
		})
	}
	return false
}

// bestAvailableAnswer returns the most recent substantive assistant message
// that is NOT a tool call and NOT a stuck placeholder. Content is stripped of
// reasoning blocks before length/placeholder checks. Skips control messages so
// injected nags/recoveries never count as the answer, and skips tool-call
// markup content — a truncated/failed <tool_call> attempt streamed as visible
// text is not an answer, and surfacing it would complete a run with raw JSON
// (observed after an upstream outage: finalizeReport's fallback returned the
// markup as the "best available answer").
func (s *runSession) bestAvailableAnswer() string {
	for i := len(s.history) - 1; i >= 0; i-- {
		m := s.history[i]
		if m.Role != proxy.AssistantRole || len(m.ToolCalls) > 0 {
			continue
		}
		if isStuckPlaceholder(m) {
			continue
		}
		if stripped := stripThinkBlocks(m.Content); len(strings.TrimSpace(stripped)) >= MinAnswerContentLength {
			if hasToolCallMarker(m.Content) || endsWithBareActionMarker(stripped) {
				continue
			}
			return stripped
		}
	}
	return ""
}

// resolveFallbackAnswer picks a deliverable when the loop cannot continue
// (hard-cap backstop only). Returns the last substantive assistant text — a
// genuine final message the model actually wrote. No synthesis, no salvage: the
// real report is surfaced only when the model actually emitted it.
func (s *runSession) resolveFallbackAnswer() string {
	return s.bestAvailableAnswer()
}

// completeWith emits lifecycle completed and returns the final answer.
// Callers that need EventMessage must notify before calling this.
func (s *runSession) completeWith(content string) (string, []proxy.Message, error) {
	s.agent.notifyLifecycle("completed", map[string]any{"content": content})
	return content, s.history, nil
}

// finalizeReport runs the deterministic tools-disabled finalization turn and
// returns the report text (or the best-available-answer fallback). It is the
// universal "produce the content" step for strategies that have no natural text
// completion — the mirror of the generatePlan pre-loop primitive (SPEC-010
// §IV.2). Shared by every strategy so the completion paths cannot drift:
// plan-execute calls it after executePlan, and the react recovery ladder
// delegates handleNoToolCalls step (2) to it. A non-empty prompt override is
// appended as the finalization instruction (defaults to AutomationFinalizePrompt);
// the caller seals the run via completeWith.
func (s *runSession) finalizeReport(ctx context.Context, prompt ...string) (string, error) {
	finalizePrompt := prompts.AutomationFinalizePrompt
	if len(prompt) > 0 && prompt[0] != "" {
		finalizePrompt = prompt[0]
	}

	s.finalizeAttempts++
	s.textOnlyNextTurn = true
	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: finalizePrompt,
	})
	s.agent.deps.Logger.Warn("forcing text-only finalization turn", "step", s.steps)

	turnMsg, parseErr, err := s.runFinalizationTurn(ctx)
	if err != nil {
		return s.finalizationFallback(err, "finalization turn")
	}

	// The finalization turn can itself be cut off by the output-token cap
	// (finish_reason="length") mid-report. Nudge a bounded continuation so the
	// report is completed instead of persisted truncated — same mechanism as
	// handleTextTurn, sharing the same counter/fragments so both paths cannot
	// exceed lengthContinuationMax combined.
	for s.maybeContinueTruncated(turnMsg, parseErr) {
		s.textOnlyNextTurn = true
		turnMsg, parseErr, err = s.runFinalizationTurn(ctx)
		if err != nil {
			return s.finalizationFallback(err, "finalization continuation")
		}
	}

	s.history = append(s.history, turnMsg)
	s.agent.notify(EventMessage, turnMsg)

	report := s.stitchTruncated(stripThinkBlocks(turnMsg.Content))
	if strings.TrimSpace(report) == "" || (parseErr != nil && parseErr.XMLFound) || hasToolCallMarker(report) || endsWithBareActionMarker(report) {
		if fallback, fbErr := s.finalizationFallback(nil, "empty finalization turn"); fallback != "" || fbErr != nil {
			return fallback, fbErr
		}
	}
	return strings.TrimSpace(report), nil
}

// runFinalizationTurn runs one LLM turn with the reasoning-stuck check
// disabled, restoring the prior setting afterwards. A thinking model may
// reason for a long stretch before writing its final report; the reasoning-
// stuck abort (designed for degenerate loops, firing at maxTokens chars of
// pure reasoning) killed finalization turns prematurely — the report never got
// written. Skip the stuck check here; the per-stream duration cap
// (streamMaxDuration) still bounds the turn.
func (s *runSession) runFinalizationTurn(ctx context.Context) (proxy.Message, *proxy.ParseError, error) {
	savedStuck := s.agent.config.SkipStuckCheck
	s.agent.config.SkipStuckCheck = true
	turnMsg, parseErr, _, err := s.agent.executeTurn(ctx, &s.history)
	s.agent.config.SkipStuckCheck = savedStuck
	return turnMsg, parseErr, err
}

// finalizationFallback recovers a deliverable when a finalization LLM turn
// fails (err != nil) or comes back unusable (empty / tool-markup / scaffold).
// Order: best available assistant answer, then a summary synthesized from the
// run's actual tool activity. A transient LLM failure must not discard
// completed work. Returns the wrapped error only when nothing recoverable
// exists (nil err → empty deliverable, so the caller can still surface the raw
// report).
func (s *runSession) finalizationFallback(err error, what string) (string, error) {
	if fallback := s.bestAvailableAnswer(); fallback != "" {
		s.agent.deps.Logger.Warn(what+" failed; using best available answer", "error", err)
		return fallback, nil
	}
	if summary := s.synthesizeRunSummary(); summary != "" {
		s.agent.deps.Logger.Warn(what+" failed; using synthesized run summary", "error", err)
		return summary, nil
	}
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", what, err)
	}
	return "", nil
}

// maybeContinueTruncated handles a content-only turn whose stream ended with
// finish_reason="length" (output-token cap): the model was cut off mid-answer,
// so this is a truncated final answer, not a completion. When the guard fires
// it keeps the partial in history, injects the bounded continuation nudge
// (mirroring Hermes's length-continuation), and returns true so the caller
// runs the next turn; the fragments are stitched at completion by
// stitchTruncated. Returns false when the turn is not truncated, attempted a
// tool call (the parse-error ladder owns that), or the continuation budget is
// exhausted.
func (s *runSession) maybeContinueTruncated(turnMsg proxy.Message, parseErr *proxy.ParseError) bool {
	if turnMsg.FinishReason != "length" || len(turnMsg.ToolCalls) > 0 {
		return false
	}
	if parseErr != nil && parseErr.XMLFound {
		return false
	}
	if s.lengthContinuationCount >= lengthContinuationMax {
		return false
	}
	// Strip once and reuse: the guard needs visible text, and the fragment
	// stored for stitching must be stripped too so the stitched report never
	// mixes stripped final content with raw reasoning blocks.
	content := stripThinkBlocks(turnMsg.Content)
	if strings.TrimSpace(content) == "" {
		return false
	}
	s.lengthContinuationCount++
	s.truncatedParts = append(s.truncatedParts, content)
	s.history = append(s.history, turnMsg)
	s.agent.notify(EventMessage, turnMsg)
	s.history = append(s.history, proxy.Message{Role: proxy.UserRole, Content: prompts.LengthContinuationPrompt})
	s.agent.deps.Logger.Warn("final answer truncated (finish_reason=length), nudging continuation",
		"content_chars", len(content),
		"continuation", s.lengthContinuationCount,
		"max", lengthContinuationMax)
	return true
}

// stitchTruncated joins the accumulated length-continuation fragments with
// content and clears the fragment buffer, so the completed answer is the full
// stitched text rather than just the last fragment (Hermes _join_truncated_parts).
func (s *runSession) stitchTruncated(content string) string {
	if len(s.truncatedParts) == 0 {
		return content
	}
	stitched := joinTruncatedParts(append(s.truncatedParts, content))
	s.truncatedParts = nil
	return stitched
}

// synthesizeRunSummary builds a degraded-but-real report from the run's actual
// tool activity when the finalization LLM turn fails (or comes back empty) and
// no assistant text exists to salvage — the plan_execute shape, where history
// is pure tool calls. A provider outage on the report turn must not discard
// completed work. Returns "" when the run did no tool work, so the caller can
// still fail loudly.
func (s *runSession) synthesizeRunSummary() string {
	perTool := make(map[string]int)
	var failures []string
	// Tool calls are counted from the usage tracker — the per-execution record
	// that survives sieving. Scanning s.history alone under-counts because the
	// physical sieve prunes history to head+tail (e.g. "2 of 18 tool calls").
	if t := GetUsageTracker(s.ctx); t != nil {
		for _, name := range t.UsedToolsSnapshot() {
			if name == models.ToolSystemError {
				continue
			}
			perTool[name]++
		}
	} else {
		// Fallback: count from history directly (unpruned runs only).
		for _, m := range s.history {
			for _, tc := range m.ToolCalls {
				if tc.Function.Name == models.ToolSystemError {
					continue
				}
				perTool[tc.Function.Name]++
			}
		}
	}
	for _, m := range s.history {
		if m.Role == proxy.ToolRole {
			if errText := toolResultError(m.Content); errText != "" {
				failures = append(failures, errText)
			}
		}
	}
	total := 0
	for _, n := range perTool {
		total += n
	}
	if total == 0 {
		return ""
	}

	names := make([]string, 0, len(perTool))
	for name := range perTool {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("Run executed tool work, but the final report generation failed (provider error). Summary synthesized from the run history:\n\n")
	b.WriteString(fmt.Sprintf("- Tool calls: %d\n", total))
	for _, name := range names {
		b.WriteString(fmt.Sprintf("  - %s × %d\n", name, perTool[name]))
	}
	if len(failures) > 0 {
		b.WriteString("- Failures recorded:\n")
		for _, f := range failures {
			b.WriteString(fmt.Sprintf("  - %s\n", truncateString(f, sessionSummaryErrorMaxLen)))
		}
	}
	b.WriteString("\nFull tool outputs are in the run recording.")
	return b.String()
}

const sessionSummaryErrorMaxLen = 240 // chars per failure line in a synthesized summary

// toolResultError extracts the "error" field from a marshaled tool result JSON
// (the shape appendToolResult produces for failures). Returns "" when the
// result is not a JSON object with a non-empty error field.
func toolResultError(content string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return ""
	}
	if e, ok := m["error"].(string); ok && strings.TrimSpace(e) != "" {
		return e
	}
	return ""
}

// truncateString shortens s to max runes with an ellipsis when longer.
func truncateString(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// joinTruncatedParts joins length-continuation fragments into the final
// answer, adding a newline where two fragments would otherwise glue together
// without whitespace. Port of Hermes Agent's _join_truncated_parts
// (agent/conversation_loop.py): continuation fragments are written to continue
// exactly where the previous one stopped, so the seam needs a separator only
// when neither side already ends/starts with whitespace.
func joinTruncatedParts(parts []string) string {
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if b.Len() > 0 && !isSpaceByte(b.String()[b.Len()-1]) && !isSpaceByte(part[0]) {
			b.WriteByte('\n')
		}
		b.WriteString(part)
	}
	return b.String()
}

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

func (s *runSession) resetParseErrorState() {
	s.starvationCount = 0
	s.parseErrorStreak = 0
	s.lastParseErrorKind = ""
	s.totalErrorStreak = 0
	s.modelCompatNotified = false
	s.syntaxParseStreak = 0
	// Re-arm the post-tool nudge: a successful tool turn means the model
	// recovered from a prior empty turn.  Clearing the counter lets a future
	// empty turn trigger a fresh nudge cycle (pre-branch behavior).
	s.postToolNudgeCount = 0
	// hardCapTriggered is left untouched — the hard cap is irreversible.
}

// maybeNudge consults the configured stop guards at the natural-completion
// branch. Returns ("", false) when finalization should proceed — either no
// guards are configured (plain react), the guard budget is exhausted, or every
// guard allows completion. Returns (nudge, true) when a guard refuses to let
// the run finalize and the caller must inject the nudge and continue the loop.
// Guards never fire on forced completion, fallback answers, or error/stall
// paths — the hook only sits on the successful-natural-completion branch.
func (s *runSession) maybeNudge() (string, bool) {
	if len(s.stopGuards) == 0 || s.stopGuardAttempts >= maxStopGuardAttempts {
		return "", false
	}
	for _, g := range s.stopGuards {
		nudge, err := g.Nudge(s)
		if err != nil {
			// A failing guard must not corrupt the run: log and allow the
			// finalization to proceed rather than failing the whole task.
			s.agent.deps.Logger.Warn("stop guard evaluation failed, allowing finalization", "error", err)
			continue
		}
		if nudge == "" {
			continue
		}
		s.stopGuardAttempts++
		return nudge, true
	}
	return "", false
}

// trimLargeWriteContent replaces write_file/append_file response content with a stub
// when the model includes a long prose preamble alongside the file write call.
// The tool result feedback is what matters — the preamble wastes context.
func (s *runSession) trimLargeWriteContent(turnMsg *proxy.Message) {
	if len(turnMsg.Content) <= sessionWriteTrimThreshold {
		return
	}
	hasWrite := false
	for _, tc := range turnMsg.ToolCalls {
		if tc.Function.Name == models.ToolFileWrite {
			hasWrite = true
			break
		}
	}
	if hasWrite {
		turnMsg.Content = fmt.Sprintf(prompts.AutomationTrimmedContentMessage, models.ToolFileWrite)
	}
}

// stripThinkBlocks removes Qwen3/Ollama <think>, <reasoning>, and
// <REASONING_SCRATCHPAD> blocks (tags + content) from a string.
// Local models that do not use structured reasoning_content may emit
// reasoning inline within <think> tags. Stripping yields the visible
// answer text. Multiple blocks are handled iteratively.
func stripThinkBlocks(content string) string {
	// Open/close pairs in matching order. Only balanced pairs are
	// removed — unclosed opening tags are left in place (they are
	// truncated output, not reasoning blocks).
	pairs := [][2]string{
		{"<think", "</think>"},
		{"<reasoning", "</reasoning>"},
		{"<reasoning_scratchpad", "</reasoning_scratchpad>"},
	}
	result := content
	for {
		changed := false
		lower := strings.ToLower(result)
		for _, p := range pairs {
			start := strings.Index(lower, p[0])
			if start == -1 {
				continue
			}
			end := strings.Index(lower[start:], p[1])
			if end == -1 {
				continue
			}
			result = result[:start] + result[start+end+len(p[1]):]
			changed = true
			break // restart with new lower string
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(result)
}

// hasOnlyHousekeepingTools returns true when every tool in the batch is
// a post-response side-effect (memory_update, etc.), not a work-horse
// tool.  Caller uses this to decide whether turn content is a final
// answer or mid-task narration.
func (s *runSession) hasOnlyHousekeepingTools(calls []proxy.ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, tc := range calls {
		switch tc.Function.Name {
		// Add housekeeping tools below when they're introduced:
		// case "memory_update", "todo":
		//     continue
		default:
			return false
		}
	}
	return true
}

// checkTaskCompletion determines if an assistant message signals task completion.
//
// A turn is complete when the assistant:
//   - Has no tool calls (not planning to act further)
//   - Produces non-empty visible content (≥MinAnswerContentLength chars after stripping reasoning tags)
//   - The content is not an unparsed tool-call attempt
//   - The run has produced at least one tool result in history (the model
//     actually did work, not just first-turn text)
//
// Unlike the Phase 2 design, completion does NOT require the immediately-
// preceding message to be a tool result. Reasoning-only turns (empty content,
// large ReasoningContent) between tool results and the final answer do not
// block completion — the gate is "any tool result in history," not adjacency.
//
// This matches Hermes Agent's proven completion model (conversation_loop.py):
// no tool calls + substantive content = done.
func checkTaskCompletion(msg proxy.Message, history []proxy.Message) (string, bool) {
	if len(msg.ToolCalls) > 0 {
		return "", false
	}

	// Strip reasoning tags — Qwen3/Ollama put <think> in content.
	// If nothing substantive remains, this is NOT a final answer.
	stripped := stripThinkBlocks(msg.Content)
	if len(stripped) < MinAnswerContentLength {
		return "", false
	}

	// Guard: text must not contain unparsed tool-call markers.
	if hasToolCallMarker(stripped) {
		return "", false
	}

	// Guard: a turn whose last line is a bare ReAct Action marker is a
	// truncated tool-call attempt — the model wrote the marker the format
	// places before every tool call, then stopped (EOS/stream end) before
	// emitting the call. That is mid-task narration, not a final answer;
	// finalizing would record an incomplete run as "completed".
	if endsWithBareActionMarker(stripped) {
		return "", false
	}

	// The run must have produced at least one tool result anywhere
	// in history (after the system prompt).  This prevents premature
	// first-turn text from being treated as completion.
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.ToolRole {
			return stripped, true
		}
	}

	return "", false
}

// hasToolResult reports whether history contains a ToolRole message. Used to
// distinguish a legitimate long final answer (real work happened) from the
// runaway tool-free joke-loop, so the no-tool content cap can be relaxed.
func hasToolResult(history []proxy.Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.ToolRole {
			return true
		}
	}
	return false
}

// isAgentControlMessage reports messages the agent injects for recovery, sieve,
// or format correction. Closed allowlist only — never match real user/task text
// by open-ended keyword heuristics. When adding a new inject site, register it here.
func isAgentControlMessage(m proxy.Message) bool {
	if isStuckPlaceholder(m) {
		return true
	}
	if m.Role != proxy.UserRole {
		return false
	}
	content := m.Content
	switch content {
	case prompts.AutomationNagPrompt,
		prompts.AutomationFinalizePrompt,
		prompts.AutomationReadFileNagPrompt,
		prompts.AutomationDuplicateNagPrompt,
		prompts.ToolErrorNagPrompt,
		prompts.ReasoningStuckNag,
		prompts.ReasoningStuckEscalatedNag,
		prompts.ContextSieveWarning,
		prompts.SieveSystemNote,
		prompts.PreSieveMemoryNudge,
		prompts.AutomationContentTooLongPrompt,
		prompts.AutomationJSONSyntaxPrompt,
		prompts.AutomationJSONPlanPrompt,
		prompts.AutomationXMLModeGuide,
		prompts.EvaluatorReviewPrompt,
		prompts.LengthContinuationPrompt:
		return true
	}
	if strings.HasPrefix(content, prompts.RetrySignal) {
		return true
	}
	// ParseError.Feedback / escalation — owned prefixes with dynamic tool lists.
	if strings.HasPrefix(content, "STOP writing text") ||
		strings.HasPrefix(content, "FORMAT ERROR:") ||
		strings.HasPrefix(content, "TOOL ERROR:") ||
		strings.HasPrefix(content, "THIRD ATTEMPT") {
		return true
	}
	return false
}

// isStuckPlaceholder is the empty/stuck assistant artifact returned by
// handleEmptyStream for native-only models (ReasoningContent "[stuck]").
func isStuckPlaceholder(m proxy.Message) bool {
	if m.Role != proxy.AssistantRole || len(m.ToolCalls) > 0 {
		return false
	}
	if strings.TrimSpace(m.Content) != "" {
		return false
	}
	return strings.TrimSpace(m.ReasoningContent) == "[stuck]"
}

// previousConversationMessage returns the nearest prior non-control message.
// Used so natural completion sees tool adjacency through recovery pollution.
// Walk is capped; if only control messages are found, returns nil (do not complete).
func previousConversationMessage(history []proxy.Message) *proxy.Message {
	limit := sessionControlWalkCap
	for i := len(history) - 1; i >= 0 && limit > 0; i-- {
		if isAgentControlMessage(history[i]) {
			limit--
			continue
		}
		return &history[i]
	}
	return nil
}

func (s *runSession) run() (string, []proxy.Message, error) {
	s.agent.runS = s
	// Only clear the back-pointer when run() exits (not panics).
	defer func() { s.agent.runS = nil }()

	// Resolve and dispatch to the configured loop strategy. The runS
	// back-pointer setup/teardown lives here (exactly once per Execute); a
	// strategy that internally delegates to another strategy (e.g. plan-execute
	// falling back to react) does not re-enter this setup.
	// Emit a one-time "working" signal before any strategy runs so the UI is
	// never blank at dispatch, even for strategies that begin with a long
	// synchronous pre-loop LLM call. Carries no content (neutral thinking
	// indicator); the loop body emits its own per-turn feedback.
	s.agent.notifyAgentThinking()

	strategy := resolveLoopStrategy(s.agent)
	return strategy.Run(s.ctx, s)
}

// generatePlan runs plan generation under a bounded per-call timeout and emits
// a "planning" UI signal first so the assistant panel is never blank during the
// synchronous pre-loop LLM call. Centralized here so every loop strategy (present
// or future) that needs a pre-loop plan gets the same timeout + feedback
// guarantees — strategies compose this primitive, never reimplement it.
func (s *runSession) generatePlan(ctx context.Context, tools []proxy.Tool, task string) (*ExecutionPlan, error) {
	s.agent.notifyAgentThinking()
	s.agent.notify(EventMessage, proxy.Message{
		Role:    "system",
		Content: MsgGeneratingPlan,
	})
	planCtx, cancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer cancel()
	strategy := NewExecutionPlanStrategy(s.agent.deps.Client, tools, s.agent.deps.Logger,
		withApplyRequest(s.agent.applyRequestConfig),
		withOnReasoning(func(reasoning string) { s.agent.notify(EventReasoning, reasoning) }),
		withOnLifecycle(func(phase string, extra map[string]any) { s.agent.notifyLifecycle(phase, extra) }),
	)
	return strategy.Generate(planCtx, task)
}

// checkForcedCompletion ends the run after MaxSteps*2 using the fallback chain.
// Returns (true, reply, err) when the loop must stop.
func (s *runSession) checkForcedCompletion() (bool, string, error) {
	if s.steps < s.agent.config.MaxSteps*2 || s.hardCapTriggered {
		return false, "", nil
	}
	s.hardCapTriggered = true
	s.agent.deps.Logger.Warn("forced completion after excessive steps",
		"maxSteps", s.agent.config.MaxSteps, "steps", s.steps)
	if content := s.resolveFallbackAnswer(); content != "" {
		reply, _, err := s.completeWith(content)
		return true, reply, err
	}
	return true, "", fmt.Errorf("agent exceeded max steps (%d) without producing a final answer", s.agent.config.MaxSteps)
}

// handleTurnError processes executeTurn failures. cont=false means return to caller.
// cont=true with nil err means continue the agent loop.
func (s *runSession) handleTurnError(err error) (done bool, reply string, outErr error) {
	s.starvationCount++
	if s.starvationCount >= DefaultStarvationLimit {
		return true, "", fmt.Errorf("agent stalled: %w", err)
	}
	if failures.IsContextSizeError(err) {
		if sieveErr := s.handleContextSizeError(); sieveErr != nil {
			return true, "", sieveErr
		}
		return false, "", nil // continue loop
	}
	if failures.IsToolCallParseError(err) {
		if !s.handleToolCallParseError(err) {
			return false, "", nil // continue loop with recovery prompt
		}
		if content := s.resolveFallbackAnswer(); content != "" {
			s.agent.deps.Logger.Warn("giving up after repeated server-side tool JSON parse errors; using fallback answer",
				"streak", s.syntaxParseStreak, "chars", len(content))
			reply, _, completeErr := s.completeWith(content)
			return true, reply, completeErr
		}
		return true, "", fmt.Errorf("agent stalled: server-side tool call JSON parse failed %d times: %w",
			s.syntaxParseStreak, err)
	}
	return true, "", err
}

// handleToolTurn runs duplicate detection, tool execution, and salvage completion.
// done=true means return from run(); done=false means continue the loop.
func (s *runSession) handleToolTurn(turnMsg proxy.Message, toolsList []proxy.Tool) (done bool, reply string, err error) {
	s.resetParseErrorState()

	isDuplicate, nagPrompt, dupErr := s.rd.check(s.agent.deps.Logger, turnMsg.ToolCalls)
	if dupErr != nil {
		return true, "", dupErr
	}
	if isDuplicate {
		nag := nagPrompt
		if len(turnMsg.ToolCalls) > 0 && turnMsg.ToolCalls[0].Function.Name == models.ToolFileRead {
			nag = prompts.AutomationReadFileNagPrompt
		}
		s.history = append(s.history, turnMsg)
		s.agent.notify(EventMessage, turnMsg)
		s.history = append(s.history, proxy.Message{
			Role:    proxy.UserRole,
			Content: nag,
		})
		return false, "", nil
	}

	if isAlternating, altErr := s.rd.checkAlternating(); isAlternating {
		return true, "", altErr
	}
	if isCycle, cycleErr := s.rd.checkSequenceRepeat(); isCycle {
		return true, "", cycleErr
	}
	if isSameTarget, tgtErr := s.rd.checkSameTarget(turnMsg.ToolCalls); isSameTarget {
		return true, "", tgtErr
	}

	s.trimLargeWriteContent(&turnMsg)

	// Content-with-tools fallback: only save when every tool in this turn
	// is housekeeping (memory, todo, etc.).  When substantive tools are
	// present (read_file, write_file, terminal, ...), the assistant text
	// is mid-task narration ("I'll scan the directory now"), not a final
	// answer.  Hermes-aligned: _last_content_with_tools only for housekeeping.
	if s.hasOnlyHousekeepingTools(turnMsg.ToolCalls) {
		if stripped := stripThinkBlocks(turnMsg.Content); len(stripped) >= MinAnswerContentLength {
			s.lastContentWithTools = stripped
		}
	}

	s.history = append(s.history, turnMsg)

	salvaged, err := s.agent.processToolCalls(s.ctx, turnMsg, &s.history, toolsList)
	if err != nil {
		return true, "", err
	}
	if salvaged != "" {
		if turnMsg.Content == "" || len(salvaged) > len(turnMsg.Content) {
			turnMsg.Content = salvaged
		}
		// The salvaged text is the agent's final answer, delivered as a
		// tool-call payload (e.g. a truncated write_file). Drop the tool
		// calls so the emitted/persisted message is a pure text reply —
		// otherwise the frontend renders a (failed) tool card and the
		// history-reconstruction logic skips the EventMessage with
		// ToolCalls, losing the report on reopen.
		turnMsg.ToolCalls = nil
		// turnMsg was appended by value above; update the persisted copy so
		// the report (and cleared ToolCalls) survive into the saved session.
		if n := len(s.history); n > 0 {
			s.history[n-1] = turnMsg
		}
		s.agent.notify(EventMessage, turnMsg)
		reply, _, completeErr := s.completeWith(salvaged)
		return true, reply, completeErr
	}

	// Tool results emitted; completion is detected on the next text turn.
	s.agent.notify(EventMessage, turnMsg)
	return false, "", nil
}

// handleTextTurn handles natural completion or no-tool recovery feedback.
func (s *runSession) handleTextTurn(turnMsg proxy.Message, parseErr *proxy.ParseError, toolsList []proxy.Tool) (done bool, reply string, err error) {
	// finish_reason="length": the model hit its output-token cap while writing
	// a content-only turn. This is a truncated final answer, NOT a complete
	// one — do not let checkTaskCompletion finalize a cut-off report. Nudge a
	// bounded continuation (mirroring Hermes's length-continuation); the
	// fragments are stitched at completion by stitchTruncated.
	if s.maybeContinueTruncated(turnMsg, parseErr) {
		return false, "", nil
	}

	// When parseErr has XMLFound, the model attempted a tool call but failed —
	// do not treat accompanying plan text as a final answer.
	if parseErr == nil || !parseErr.XMLFound {
		if content, ok := checkTaskCompletion(turnMsg, s.history); ok {
			s.history = append(s.history, turnMsg)
			s.agent.notify(EventMessage, turnMsg)
			// Stop-guard hook (evaluator-optimizer): a guard may refuse to let
			// the run finalize, injecting a self-review nudge to continue the
			// loop. Fires only on successful natural completion — never on
			// forced completion, fallback answers, or error/stall returns.
			// With no guards configured maybeNudge always allows finalization.
			if nudge, guardNudged := s.maybeNudge(); guardNudged {
				s.history = append(s.history, proxy.Message{Role: proxy.UserRole, Content: nudge})
				return false, "", nil
			}
			// A truncated final answer was completed by one or more
			// length-continuation turns — stitch the fragments so the report
			// is the full text, not just the last fragment (Hermes
			// _join_truncated_parts). Runs after the guard check so a
			// guard-nudged continuation keeps the fragments for a later stitch.
			reply, _, completeErr := s.completeWith(s.stitchTruncated(content))
			return true, reply, completeErr
		}
	}

	s.starvationCount++
	if s.starvationCount >= DefaultStarvationLimit {
		return true, "", fmt.Errorf("agent stalled: no tool calls in %d consecutive turns", s.starvationCount)
	}
	reply, shouldExit, err := s.handleNoToolCalls(turnMsg, parseErr, toolsList)
	if err != nil {
		return true, "", err
	}
	if shouldExit {
		// Terminal paths return a real deliverable (finalization report,
		// fallback answer, premature-termination text). Seal via the shared
		// completion path so the "completed" lifecycle fires exactly once on
		// success (SPEC-010 §V), matching the natural-completion branch.
		reply, _, completeErr := s.completeWith(reply)
		return true, reply, completeErr
	}
	return false, "", nil
}

// executeTurn runs one LLM call, parses tool calls from content, validates
// them, and deduplicates.  A non-nil parseErr means the model produced
// malformed XML/native tool calls — the caller decides whether to escalate.
func (a *Agent) executeTurn(ctx context.Context, history *[]proxy.Message) (proxy.Message, *proxy.ParseError, []proxy.Tool, error) {
	// Guardrail-blocked tool loop guard: a model that keeps re-calling a
	// blocked tool (each attempt with new arguments) never triggers the
	// args-keyed spiral detector. Once the denial streak crosses the limit,
	// inject a targeted "stop using this tool" nag and raise the sampling
	// temperature for this turn to break the rut. One-shot per streak (re-armed
	// on any allowed tool call); skipped on the tools-disabled finalization
	// turn where no tools run.
	if a.runS != nil && !a.runS.textOnlyNextTurn && !a.runS.guardrailNagSent &&
		a.runS.guardrailBlockStreak >= guardrailBlockStreakLimit {
		a.runS.guardrailNagSent = true
		a.runS.recoveryTempEscalation += recoveryTempStep
		if a.runS.recoveryTempEscalation > maxRecoveryTemp {
			a.runS.recoveryTempEscalation = maxRecoveryTemp
		}
		tool := a.runS.guardrailBlockedTool
		if tool == "" {
			tool = "that tool"
		}
		*history = append(*history, proxy.Message{
			Role:    proxy.UserRole,
			Content: fmt.Sprintf(prompts.GuardrailBlockedNagPrompt, tool),
		})
		a.notifyGuardrailLoopBlocked(tool)
		a.deps.Logger.Warn("guardrail-blocked tool loop: nagging model to switch tools",
			"tool", tool, "streak", a.runS.guardrailBlockStreak, "temp", a.runS.recoveryTempEscalation)
	}

	turnCtx, turnCancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer turnCancel()

	toolsList, err := a.deps.Provider.ListTools(turnCtx)
	if err != nil {
		return proxy.Message{}, nil, nil, fmt.Errorf("failed to list tools: %w", err)
	}

	// The physical sieve must measure the REAL request size — the prepared
	// messages include the system-prompt enrichment (tool reference/manual,
	// hot memory) that raw history omits. Measuring only raw history
	// under-counts the request by several KB, so the sieve could fire too late
	// and let the prompt overflow the serving context (2026-08-30 smoke-test
	// run: context grew to ~27.7 KB with the sieve never firing, then the
	// server 400'd "context too long").
	if a.preparedOverContextBudget(*history, toolsList) {
		*history = a.applyPhysicalSieve(*history)
	}

	// Finalization turn: run ONE turn with tools disabled so the model is
	// forced to deliver a text report. Reset immediately so it fires once.
	toolChoice := proxy.ToolChoice("")
	if a.runS != nil && a.runS.textOnlyNextTurn {
		toolsList = nil
		toolChoice = proxy.ToolChoiceNone
		a.runS.textOnlyNextTurn = false
	}

	msg, err := a.computeNextResponse(turnCtx, *history, toolsList, toolChoice)
	if err != nil {
		return proxy.Message{}, nil, nil, err
	}

	a.deps.Logger.Debug("raw model response",
		"content_len", len(msg.Content),
		"content", msg.Content,
		"native_tool_calls", len(msg.ToolCalls),
	)

	parseErr := a.handleContentToolCalls(&msg)
	turnMsg := msg

	// Only warn when the model genuinely attempted a tool call and it was
	// malformed (XMLFound=true). A plain-text turn with no tool-call markers
	// and no native tool calls is a normal final answer — not a parse error.
	// The spurious WARN previously fired on every final report.
	if parseErr != nil && parseErr.XMLFound {
		contentPreview := ""
		if len(msg.Content) > 0 {
			contentPreview = msg.Content
			if len(contentPreview) > sessionPreviewMaxLen {
				contentPreview = contentPreview[:sessionPreviewMaxLen]
			}
		}
		a.deps.Logger.Warn("tool call parse error",
			"xml_found", parseErr.XMLFound,
			"json_error", parseErr.JSONError,
			"tool_name", parseErr.ToolName,
			"content_preview", contentPreview,
		)
	}

	if len(turnMsg.ToolCalls) > 0 {
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
			turnMsg.ToolCalls = nil
			if len(turnMsg.Content) > sessionTruncationFeedbackLen && failures.IsTruncationError(parseErr.JSONError) {
				turnMsg.Content = "[Large response truncated — see next message for guidance.]"
			}
			return turnMsg, parseErr, toolsList, nil
		}

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
	}

	return turnMsg, parseErr, toolsList, nil
}

func (s *runSession) handleNoToolCalls(
	turnMsg proxy.Message,
	parseErr *proxy.ParseError,
	toolsList []proxy.Tool,
) (string, bool, error) {
	if len(s.history) == 0 || s.history[len(s.history)-1].Content != turnMsg.Content {
		s.history = append(s.history, turnMsg)
		s.agent.notify(EventMessage, turnMsg)
	}

	if s.agent.isPrematureTermination(turnMsg, s.history) {
		s.agent.deps.Logger.Warn("premature termination detected", "step", s.steps)
		return turnMsg.Content, true, nil
	}

	if parseErr != nil {
		reply, done := s.handleParseErrorFeedback(parseErr, toolsList, turnMsg.Content)
		if done {
			return reply, true, nil
		}
	}

	if len(toolsList) == 0 && strings.TrimSpace(turnMsg.Content) != "" {
		return turnMsg.Content, true, nil
	}

	// Genuinely no content: empty turn, possibly after tool results.
	// Check content-with-tools fallback first — the model may have
	// written its answer alongside the previous tool calls.
	if s.lastContentWithTools != "" {
		content := s.lastContentWithTools
		s.lastContentWithTools = ""
		s.agent.deps.Logger.Info("using content-with-tools fallback as final answer",
			"chars", len(content))
		return content, true, nil
	}

	// Empty-turn recovery ladder (model-agnostic, bounded, re-arming).
	// A model that returns an empty turn after doing work is nudged, then
	// forced into one tools-disabled finalization turn, then terminated with
	// the best answer available. The nudge counter is cleared on every
	// successful tool turn (resetParseErrorState), so this re-arms.

	// (1) Re-armed nudge: inject the nag prompt up to postToolNudgeMax times.
	if s.postToolNudgeCount < postToolNudgeMax {
		s.postToolNudgeCount++
		s.history = append(s.history, proxy.Message{
			Role:    proxy.UserRole,
			Content: prompts.AutomationNagPrompt,
		})
		s.agent.deps.Logger.Warn("no tool calls - re-arming nudge",
			"step", s.steps, "nudge", s.postToolNudgeCount)
		return "", false, nil
	}

	// (2) Deterministic finalization: force ONE text-only turn (tools disabled).
	// Delegated to finalizeReport — the shared primitive plan-execute also uses —
	// so the two completion paths cannot drift. finalizeAttempts is exhausted →
	// step (3) terminal. Exactly one finalization turn — no loop.
	if s.finalizeAttempts < 1 {
		report, err := s.finalizeReport(s.ctx)
		if err != nil {
			return "", false, err
		}
		return report, true, nil
	}

	// (3) Terminal: best real answer we have, or an honest empty note.
	if content := s.bestAvailableAnswer(); content != "" {
		return content, true, nil
	}
	return "", true, nil
}

// handleParseErrorFeedback handles the parse-error branch of handleNoToolCalls.
// When XMLFound is false and the message has substantive content, it trusts the
// model's completion. Otherwise, it injects parse-error guidance feedback.
func (s *runSession) handleParseErrorFeedback(parseErr *proxy.ParseError, toolsList []proxy.Tool, content string) (string, bool) {
	if !parseErr.XMLFound && strings.TrimSpace(content) != "" {
		// A turn ending on a bare ReAct Action marker line is a truncated
		// tool-call attempt — never trust it as a completion.
		if !endsWithBareActionMarker(content) {
			return content, true
		}
	}
	if len(toolsList) == 0 {
		return "", true
	}
	s.totalErrorStreak++
	if s.totalErrorStreak >= sessionModelCompatNotifyAfter && !s.modelCompatNotified {
		s.modelCompatNotified = true
		s.agent.notifyModelCompatWarning(s.agent.config.UseNativeTools)
	}
	availableNames := proxy.AvailableToolNames(toolsList)
	feedback := parseErr.Feedback(availableNames)
	s.agent.deps.Logger.Debug("injecting parse-error feedback", "error", parseErr.Error(), "feedback", feedback)
	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: feedback,
	})
	return "", false
}

// endsWithBareActionMarker reports whether the last non-empty line of content
// is exactly the ReAct Action delimiter ("Action:"). The system prompt's own
// Thought -> Action contract places a bare "Action:" line immediately before
// every tool call; content that ends there means the generation stopped (EOS)
// before the tool call was emitted — an incomplete turn, never a final report.
// Matching the full line — not a word suffix — keeps a legitimate final
// report that merely mentions "...the action:" from being rejected.
func endsWithBareActionMarker(content string) bool {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return strings.EqualFold(line, "action:")
	}
	return false
}

// hasToolCallMarker checks whether content contains XML-like markers that
// indicate the model was attempting a tool call, even when neither parser
// found a complete, parseable call (e.g., stream truncation before the
// closing tag left an incomplete <tool_call or <function block).
// Markers are matched case-insensitively, mirroring the regex parsers'
// (?is) flags.
func hasToolCallMarker(content string) bool {
	lower := strings.ToLower(content)
	markers := []string{
		"<tool_call", "</tool_call",
		"<function", "</function",
		"<tool ", "<tool>",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// handleContentToolCalls tries XML parsing first (broader coverage), then
// native format if the model is in native-tools mode.  Returns the first
// ParseError if both fail — the caller feeds it back as guidance.
func (a *Agent) handleContentToolCalls(msg *proxy.Message) *proxy.ParseError {
	if msg.Content == "" {
		return nil
	}
	cleaned, calls, parseErr := proxy.ParseContentToolCalls(msg.Content)
	msg.Content = cleaned
	if parseErr == nil && len(calls) > 0 {
		msg.ToolCalls = append(msg.ToolCalls, calls...)
		a.deps.Logger.Debug("xml tool calls parsed from content", "count", len(calls))
		return nil
	}

	// Native-format fallback. Previously gated on UseNativeTools, but a model
	// that natively emits <function=name><parameter>… text does so REGARDLESS of
	// mode — e.g. when the capability probe failed (server loading/restarting)
	// and the run fell back to XML text mode, a native-format model kept
	// writing <function=…> blocks and every turn died with a parse error
	// (2026-08-31 smoke-test run). Try the native parser as a fallback in
	// every mode; it only matches the specific native tags, so it cannot
	// misfire on conversational text.
	{
		nativeCleaned, nativeCalls, nativeErr := proxy.ParseNativeToolCalls(msg.Content)
		if nativeErr == nil && len(nativeCalls) > 0 {
			msg.Content = nativeCleaned
			msg.ToolCalls = append(msg.ToolCalls, nativeCalls...)
			a.deps.Logger.Debug("native format tool calls parsed from content", "count", len(nativeCalls))
			return nil
		}
		// Prefer the native parser's error when it found tags (XMLFound=true)
		// but the XML parser didn't — it means the model used native format
		// tags (<function name=...>) rather than XML format (<tool_call>).
		if nativeErr != nil && nativeErr.XMLFound && !parseErr.XMLFound {
			return nativeErr
		}
	}

	// Both parsers failed and reported XMLFound=false, but the content has
	// visible tool-call markers (e.g. truncated <tool_call or <function).
	// The model was attempting a call — don't let the preceding text be
	// treated as a natural completion.
	if parseErr != nil && !parseErr.XMLFound && hasToolCallMarker(msg.Content) {
		return &proxy.ParseError{XMLFound: true, JSONError: proxy.ErrMsgIncompleteToolCall}
	}

	return parseErr
}

func (a *Agent) precededByToolResult(history []proxy.Message) bool {
	if len(history) < 2 {
		return false
	}
	last := history[len(history)-1]
	if last.Role != proxy.AssistantRole ||
		len(last.ToolCalls) > 0 ||
		strings.TrimSpace(last.Content) == "" {
		return false
	}
	prev := previousConversationMessage(history[:len(history)-1])
	return prev != nil && prev.Role == proxy.ToolRole
}

func (a *Agent) countConsecutiveChat(history []proxy.Message) int {
	chatCount := 0
	var lastContent string
	for j := len(history) - 1; j >= 0; j-- {
		if history[j].Role == proxy.AssistantRole && len(history[j].ToolCalls) == 0 {
			current := strings.TrimSpace(history[j].Content)
			if lastContent != "" && len(current) > sessionMinMonologueLen && strings.Contains(lastContent, current[:len(current)/2]) {
				chatCount += 2
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

func (a *Agent) isPrematureTermination(msg proxy.Message, history []proxy.Message) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		// Reasoning-only is NOT premature — the model is thinking and should be
		// nagged to produce an action, not terminated.
		if len(msg.ReasoningContent) > 0 {
			return false
		}
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

func (s *runSession) maybeFlushMemoryBeforeTurn() {
	if s.agent.deps.MemoryStore == nil || s.memoryFlushSent || !s.agent.config.EnableHotMemory {
		return
	}

	totalChars := 0
	for _, m := range s.history {
		totalChars += len(m.Content)
	}
	if totalChars == 0 || s.agent.config.ContextBudget == 0 {
		return
	}

	ratio := float64(totalChars) / float64(s.agent.config.ContextBudget)
	if ratio < MemoryFlushRatio {
		return
	}

	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.PreSieveMemoryNudge,
	})
	s.memoryFlushSent = true
}
