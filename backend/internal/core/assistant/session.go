// session.go — runSession struct (encapsulates one Execute call), turn
// execution, no-tool-call heuristics, content tool call parsing, termination
// detection, and the main agentic loop body.
package assistant

import (
	"context"
	"fmt"
	"strings"

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

	MinAnswerContentLength = 20   // chars threshold for meaningful assistant content
	MemoryFlushRatio       = 0.7  // context budget fraction that triggers memory flush
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

	parseErrorStreak       int
	lastParseErrorKind     string
	totalErrorStreak       int
	modelCompatNotified    bool
	forcedCompletionSent   bool
	syntaxParseStreak      int // consecutive server-side tool-arg JSON syntax failures

	rd              repetitionDetector
	memoryFlushSent     bool   // prevents repeated pre-sieve nudges across turns
	lastContentWithTools string // content saved from a turn that had both text and tool calls

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
	case isJSONSyntaxError(err):
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

// bestAvailableAnswer returns the most recent assistant content with len ≥ MinAnswerContentLength.
func (s *runSession) bestAvailableAnswer() string {
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].Role == proxy.AssistantRole {
			if c := strings.TrimSpace(s.history[i].Content); len(c) >= MinAnswerContentLength {
				return s.history[i].Content
			}
		}
	}
	return ""
}

// lastNonEmptyAssistantContent returns the newest assistant message with any
// non-empty body (used as last-resort forced completion).
func (s *runSession) lastNonEmptyAssistantContent() string {
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].Role == proxy.AssistantRole {
			if c := strings.TrimSpace(s.history[i].Content); c != "" {
				return s.history[i].Content
			}
		}
	}
	return ""
}

// resolveFallbackAnswer picks a deliverable when the loop cannot continue.
// Priority: salvaged write report → meaningful assistant text → any assistant text.
func (s *runSession) resolveFallbackAnswer() string {
	if report := salvageReportFromHistory(s.history); report != "" {
		return report
	}
	if content := s.bestAvailableAnswer(); content != "" {
		return content
	}
	return s.lastNonEmptyAssistantContent()
}

// completeWith emits lifecycle completed and returns the final answer.
// Callers that need EventMessage must notify before calling this.
func (s *runSession) completeWith(content string) (string, []proxy.Message, error) {
	s.agent.notifyLifecycle("completed", map[string]any{"content": content})
	return content, s.history, nil
}

func (s *runSession) resetParseErrorState() {
	s.starvationCount = 0
	s.parseErrorStreak = 0
	s.lastParseErrorKind = ""
	s.totalErrorStreak = 0
	s.modelCompatNotified = false
	s.syntaxParseStreak = 0
	// Hermes-aligned: successful tool execution means the model
	// recovered from a prior empty-turn nag.  Clear the one-shot
	// nag flag so a future empty turn can trigger a new nag cycle.
	s.forcedCompletionSent = false
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
		prompts.AutomationXMLModeGuide:
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

	for {
		s.steps++
		if err := s.ctx.Err(); err != nil {
			return "", s.history, fmt.Errorf("agent execution halted: %w", err)
		}

		if done, reply, err := s.checkForcedCompletion(); done {
			return reply, s.history, err
		}

		if s.steps >= s.agent.config.MaxSteps && !s.warnedAdvisory {
			s.warnedAdvisory = true
			s.agent.deps.Logger.Warn("agent exceeded advisory step limit, continuing", "steps", s.steps)
		}

		s.maybeFlushMemoryBeforeTurn()

		s.agent.notifyStepStart(s.steps)
		s.agent.notifyThinking()

		turnMsg, parseErr, toolsList, err := s.agent.executeTurn(s.ctx, &s.history)
		if err != nil {
			done, reply, turnErr := s.handleTurnError(err)
			if done {
				return reply, s.history, turnErr
			}
			if turnErr != nil {
				return "", s.history, turnErr
			}
			continue
		}

		s.sieveStreak = 0

		if len(turnMsg.ToolCalls) > 0 {
			done, reply, turnErr := s.handleToolTurn(turnMsg, toolsList)
			if done {
				return reply, s.history, turnErr
			}
			if turnErr != nil {
				return "", s.history, turnErr
			}
			continue
		}

		done, reply, turnErr := s.handleTextTurn(turnMsg, parseErr, toolsList)
		if done {
			return reply, s.history, turnErr
		}
		if turnErr != nil {
			return "", s.history, turnErr
		}
	}
}

// checkForcedCompletion ends the run after MaxSteps*2 using the fallback chain.
// Returns (true, reply, err) when the loop must stop.
func (s *runSession) checkForcedCompletion() (bool, string, error) {
	if s.steps < s.agent.config.MaxSteps*2 || s.forcedCompletionSent {
		return false, "", nil
	}
	s.forcedCompletionSent = true
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
	if isContextSizeError(err) {
		if sieveErr := s.handleContextSizeError(); sieveErr != nil {
			return true, "", sieveErr
		}
		return false, "", nil // continue loop
	}
	if isToolCallParseError(err) {
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
	// When parseErr has XMLFound, the model attempted a tool call but failed —
	// do not treat accompanying plan text as a final answer.
	if parseErr == nil || !parseErr.XMLFound {
		if content, ok := checkTaskCompletion(turnMsg, s.history); ok {
			s.history = append(s.history, turnMsg)
			s.agent.notify(EventMessage, turnMsg)
			reply, _, completeErr := s.completeWith(content)
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
		return true, reply, nil
	}
	return false, "", nil
}

// executeTurn runs one LLM call, parses tool calls from content, validates
// them, and deduplicates.  A non-nil parseErr means the model produced
// malformed XML/native tool calls — the caller decides whether to escalate.
func (a *Agent) executeTurn(ctx context.Context, history *[]proxy.Message) (proxy.Message, *proxy.ParseError, []proxy.Tool, error) {
	turnCtx, turnCancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer turnCancel()

	toolsList, err := a.deps.Provider.ListTools(turnCtx)
	if err != nil {
		return proxy.Message{}, nil, nil, fmt.Errorf("failed to list tools: %w", err)
	}

	*history = a.applyPhysicalSieve(*history)

	msg, err := a.computeNextResponse(turnCtx, *history, toolsList)
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

	if parseErr != nil && (parseErr.XMLFound || len(turnMsg.ToolCalls) == 0) {
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
			if len(turnMsg.Content) > sessionTruncationFeedbackLen && isTruncationError(parseErr.JSONError) {
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

	// One-shot nag: inject the nag prompt ONCE.  Previously this loop
	// nagged perpetually — a model that returns empty after tools gets
	// one retry, then the fallback or forced-completion path (via
	// checkForcedCompletion at MaxSteps*2) finishes the run.  This
	// matches Hermes Agent's post-tool empty-response nudge behavior.
	if s.forcedCompletionSent {
		// Already nagged once.  Use the full fallback chain which
		// includes salvageReportFromHistory (recovers write_file
		// content from tool-call args) and bestAvailableAnswer.
		if content := s.resolveFallbackAnswer(); content != "" {
			return content, true, nil
		}
		return "", true, nil
	}
	s.forcedCompletionSent = true
	s.agent.deps.Logger.Warn("no tool calls - nagging model (one-shot)", "step", s.steps)
	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.AutomationNagPrompt,
	})
	return "", false, nil
}

// handleParseErrorFeedback handles the parse-error branch of handleNoToolCalls.
// When XMLFound is false and the message has substantive content, it trusts the
// model's completion. Otherwise, it injects parse-error guidance feedback.
func (s *runSession) handleParseErrorFeedback(parseErr *proxy.ParseError, toolsList []proxy.Tool, content string) (string, bool) {
	if !parseErr.XMLFound && strings.TrimSpace(content) != "" {
		return content, true
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

	if a.config.UseNativeTools {
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
