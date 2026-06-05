// session.go — runSession struct (encapsulates one Execute call), turn
// execution, no-tool-call heuristics, content tool call parsing, termination
// detection, and the main agentic loop body.
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

const (
	sessionMaxSieveRetries         = 3    // consecutive context-overflow errors before giving up
	sessionAutoAdvanceThreshold    = 3    // turns without complete_step before auto-advancing PlanState
	sessionMinHistoryForSieve      = 2    // fewer messages and the sieve can't operate
	sessionModelCompatNotifyAfter  = 5    // consecutive parse errors before suggesting model swap
	sessionWriteTrimThreshold      = 1000 // chars above which write_file content is replaced with a stub
	sessionPreviewMaxLen           = 500  // chars for parse-error content preview in log
	sessionTruncationFeedbackLen   = 400  // chars above which truncated content gets a guidance message
	sessionParseErrorEscalation    = 2    // same-error-kind streak before escalating feedback
	sessionConsecutiveChatExit     = 2    // non-automation: exit after N consecutive chat-only turns
	sessionMinMonologueLen         = 10   // chars below which repetition check is skipped
)

// failTracker tracks tool execution errors across turns and injects
// guidance when a tool fails repeatedly.  Two independent counters:
//   - exactFailures: same tool + same normalized args
//   - sameToolFailures: same tool name (any args)
// Counters are cleared on success (Hermes-style "clear on progress").
type failTracker struct {
	exactFailures    map[string]int
	sameToolFailures map[string]int
}

func newFailTracker() *failTracker {
	return &failTracker{
		exactFailures:    make(map[string]int),
		sameToolFailures: make(map[string]int),
	}
}

func (ft *failTracker) record(toolName, argsHash string, isError bool) string {
	if isError {
		exactKey := toolName + "\x00" + argsHash
		ft.exactFailures[exactKey]++
		ft.sameToolFailures[toolName]++
		return prompts.ToolFailureGuidance(toolName, ft.exactFailures[exactKey], ft.sameToolFailures[toolName])
	}

	delete(ft.sameToolFailures, toolName)
	for k := range ft.exactFailures {
		if strings.HasPrefix(k, toolName+"\x00") {
			delete(ft.exactFailures, k)
		}
	}
	return ""
}

// runSession encapsulates the mutable state of one Agent.Execute call.
// Fields replace the old pointer-to-primitive pattern (was *int, *bool, *string).
type runSession struct {
	agent   *Agent
	ctx     context.Context

	history        []proxy.Message
	steps          int
	sieveStreak    int
	starvationCount int
	warnedAdvisory bool

	parseErrorStreak    int
	lastParseErrorKind  string
	totalErrorStreak    int
	modelCompatNotified bool

	isAutomation     bool
	rd               repetitionDetector
	memoryFlushSent  bool   // prevents repeated pre-sieve nudges across turns
	ft               *failTracker

	turnsOnCurrentStep int  // consecutive tool-call turns without complete_step
}

func newRunSession(agent *Agent, ctx context.Context, history []proxy.Message) *runSession {
	s := &runSession{
		agent:   agent,
		ctx:     ctx,
		history: append([]proxy.Message{}, history...),
		ft:      newFailTracker(),
	}
	for _, m := range s.history {
		if prompts.IsAutomationTask(m.Content) {
			s.isAutomation = true
			break
		}
	}
	return s
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

func (s *runSession) handleToolCallParseError() {
	s.agent.logger.Warn("server-side tool call JSON parse error, sending length feedback to model")
	s.totalErrorStreak++
	if s.totalErrorStreak >= sessionModelCompatNotifyAfter && !s.modelCompatNotified {
		s.modelCompatNotified = true
		s.agent.notifyModelCompatWarning(s.agent.useNativeTools)
	}
	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.AutomationContentTooLongPrompt,
	})
}

func (s *runSession) resetParseErrorState() {
	s.starvationCount = 0
	s.parseErrorStreak = 0
	s.lastParseErrorKind = ""
	s.totalErrorStreak = 0
	s.modelCompatNotified = false
}

// trimLargeWriteContent replaces write_file response content with a stub
// when the model includes a long prose preamble alongside a write_file call.
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
		turnMsg.Content = "[Response trimmed — write_file content too long. See tool result feedback.]"
	}
}

func (s *runSession) checkSubmitFinalAnswer(turnMsg proxy.Message) (string, bool) {
	for _, tc := range turnMsg.ToolCalls {
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			summary := extractTaskSummary(tc.Function.Arguments)
			if turnMsg.Content == "" || (summary != "Task complete." && summary != "") {
				turnMsg.Content = summary
			}
			return turnMsg.Content, true
		}
	}
	return "", false
}

func (s *runSession) run() (string, []proxy.Message, error) {
	for {
		s.steps++
		if err := s.ctx.Err(); err != nil {
			return "", s.history, fmt.Errorf("agent execution halted: %w", err)
		}

		if s.steps >= s.agent.maxSteps && !s.warnedAdvisory {
			s.warnedAdvisory = true
			s.agent.logger.Warn("agent exceeded advisory step limit, continuing", "steps", s.steps)
		}

		s.maybeFlushMemoryBeforeTurn()

		s.agent.notifyStepStart(s.steps)
		s.agent.notifyThinking()

		historyLenBefore := len(s.history)
		turnMsg, parseErr, toolsList, err := s.agent.executeTurn(s.ctx, &s.history)
		if err != nil {
			s.starvationCount++
			if s.starvationCount >= DefaultStarvationLimit {
				return "", s.history, fmt.Errorf("agent stalled: %w", err)
			}
			if isContextSizeError(err) {
				err := s.handleContextSizeError()
				if err != nil {
					return "", s.history, err
				}
				continue
			}

			if isToolCallParseError(err) {
				s.handleToolCallParseError()
				continue
			}

			return "", s.history, err
		}

		s.sieveStreak = 0

		if s.memoryFlushSent && len(s.history) < historyLenBefore {
			s.memoryFlushSent = false
		}

		if len(turnMsg.ToolCalls) > 0 {
			s.resetParseErrorState()

			isDuplicate, nagPrompt, dupErr := s.rd.check(s.agent.logger, turnMsg.ToolCalls)
			if dupErr != nil {
				return "", s.history, dupErr
			}
			if isDuplicate {
				s.history = append(s.history, turnMsg)
				s.agent.notify(EventMessage, turnMsg)
				for _, tc := range turnMsg.ToolCalls {
					if tc.Function.Name == models.ToolSubmitFinalAnswer || tc.Function.Name == models.ToolSystemError {
						continue
					}
					if _, lastResult := findLastToolResultByToolName(s.history, tc.Function.Name); lastResult != nil && isToolErrorResult(lastResult.Content) {
						argsHash := normalizeArgs(tc.Function.Arguments)
						if guidance := s.ft.record(tc.Function.Name, argsHash, true); guidance != "" {
							lastResult.Content += "\n\n" + guidance
						}
					}
				}
				s.history = append(s.history, proxy.Message{
					Role:    proxy.UserRole,
					Content: nagPrompt,
				})
				continue
			}

			s.trimLargeWriteContent(&turnMsg)

			s.history = append(s.history, turnMsg)
			s.agent.notify(EventMessage, turnMsg)
			s.interceptRedundantToolCalls(&turnMsg, &s.history)
			if err := s.agent.processToolCalls(s.ctx, turnMsg, &s.history); err != nil {
				return "", s.history, err
			}
			s.applyFailGuidance(turnMsg)

			// Handle complete_step — advance the execution state
			hasCompleteStep := false
			for _, tc := range turnMsg.ToolCalls {
				if tc.Function.Name == models.ToolCompleteStep {
					hasCompleteStep = true
					if s.agent.state != nil && s.agent.state.ConfirmOrCompleteStep() {
						s.agent.logger.Info("step completed", "command", tc.Function.Arguments)
					}
				}
				if tc.Function.Name == models.ToolTerminalExecute {
					if lastResult := findToolResultByID(s.history, tc.ID); lastResult != nil {
						s.cacheToolResult(tc, lastResult.Content)
					}
				}
			}

			// Auto-advance PlanState when the model goes multiple turns without
			// calling complete_step.  Prevents step-regression in smaller models
			// that forget to call it reliably.
			if s.agent.state != nil {
				if hasCompleteStep {
					s.turnsOnCurrentStep = 0
				} else if len(turnMsg.ToolCalls) > 0 {
					s.turnsOnCurrentStep++
					if s.turnsOnCurrentStep >= sessionAutoAdvanceThreshold {
						if s.agent.state.AutoAdvanceActiveStep() {
							s.agent.logger.Info("step auto-advanced (no complete_step call)", "turns", s.turnsOnCurrentStep)
							s.history = append(s.history, proxy.Message{
								Role:    proxy.SystemRole,
								Content: "[Step auto-advanced. The model did not call complete_step for the previous step. Continue with the next step.]",
							})
						}
						s.turnsOnCurrentStep = 0
					}
				}
			}

			// Non-streaming retry for submit_final_answer when the model exhausts
			// its reasoning budget and produces truncated JSON.  The retry runs
			// in text mode (no tool_choice) so the model sees its own reasoning
			// in history and generates a clean tool call with the full report.
			if s.isAutomation && needsSubmitRetry(s.history, turnMsg) {
				// First check if we can recover the summary from the truncated JSON
				recoverable := false
				for _, tc := range turnMsg.ToolCalls {
					if tc.Function.Name == models.ToolSubmitFinalAnswer {
						summary := extractTaskSummary(tc.Function.Arguments)
						recoverable = summary != "Task complete." && summary != ""
						break
					}
				}
				if !recoverable {
					retryMsg, retryErr := s.retrySubmitWithChat()
					if retryErr == nil && len(retryMsg.ToolCalls) > 0 {
						for _, tc := range retryMsg.ToolCalls {
							if tc.Function.Name == models.ToolSubmitFinalAnswer {
								turnMsg = retryMsg
								s.agent.appendToolResult(&s.history, tc, map[string]string{"result": "Task submitted successfully."})
								break
							}
						}
					}
				}
			}

			if content, done := s.checkSubmitFinalAnswer(turnMsg); done {
				return content, s.history, nil
			}
		} else {
			s.starvationCount++
			if s.starvationCount >= DefaultStarvationLimit {
				return "", s.history, fmt.Errorf("agent stalled: no tool calls in %d consecutive turns", s.starvationCount)
			}
			reply, shouldExit, err := s.handleNoToolCalls(
				turnMsg,
				parseErr,
				toolsList,
			)
			if err != nil {
				return "", s.history, err
			}
			if shouldExit {
				return reply, s.history, nil
			}
		}
	}
}

// interceptRedundantToolCalls checks each terminal command against the LRU
// tool result cache. When the exact same command string (plus CWD) has been
// executed before in this session, the tool is pre-empted and a synthetic
// result containing the cached output is injected. The model sees the output
// and continues; it never knows the tool was skipped.
func (s *runSession) interceptRedundantToolCalls(turnMsg *proxy.Message, history *[]proxy.Message) {
	cache := s.agent.toolCache
	if cache == nil {
		return
	}

	var remaining []proxy.ToolCall
	for _, tc := range turnMsg.ToolCalls {
		if tc.Function.Name != models.ToolTerminalExecute {
			remaining = append(remaining, tc)
			continue
		}

		if cached, ok := cache.Get(tc.Function.Arguments); ok {
			s.agent.logger.Info("tool result cache hit",
				"tool", tc.Function.Name, "command", tc.Function.Arguments)
			s.agent.appendToolResult(history, tc, map[string]string{"stdout": cached, "stderr": ""})
			continue
		}

		remaining = append(remaining, tc)
	}
	turnMsg.ToolCalls = remaining
}

// needsSubmitRetry returns true when submit_final_answer was in the turn's
// tool calls but its arguments failed JSON validation — the model ran out
// of reasoning budget and produced truncated JSON that couldn't be parsed.
func needsSubmitRetry(history []proxy.Message, turnMsg proxy.Message) bool {
	for _, tc := range turnMsg.ToolCalls {
		if tc.Function.Name != models.ToolSubmitFinalAnswer {
			continue
		}
		if lastResult := findToolResultByID(history, tc.ID); lastResult != nil {
			if strings.Contains(lastResult.Content, "INVALID ARGUMENTS") {
				return true
			}
		}
		return false
	}
	return false
}

// retrySubmitWithChat sends a non-streaming Chat request when the model's
// submit_final_answer had truncated JSON.  The retry sanitises history to
// remove the malformed tool call and uses text mode (no tools) so the server
// does not reject the request and the model generates clean content that the
// XML parser can extract.  Returns the parsed message or an error.
func (s *runSession) retrySubmitWithChat() (proxy.Message, error) {
	s.agent.logger.Info("retrying submit_final_answer with non-streaming Chat")

	nag := proxy.Message{
		Role: proxy.UserRole,
		Content: "Your submission had a formatting error. " +
			"Resubmit the complete report using submit_final_answer with valid JSON.",
	}
	s.history = append(s.history, nag)

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	sanitized := sanitizeHistoryForRetry(s.history)

	req := proxy.ChatRequest{
		Messages:  sanitized,
		Tools:     nil,
		MaxTokens: s.agent.maxTokens * 2,
	}

	resp, err := s.agent.client.Chat(ctx, req)
	s.history = s.history[:len(s.history)-1]

	if err != nil {
		return proxy.Message{}, fmt.Errorf("retry chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return proxy.Message{}, fmt.Errorf("retry chat: empty response")
	}

	msg := resp.Choices[0].Message

	for _, tc := range msg.ToolCalls {
		if tc.Function.Name == models.ToolSubmitFinalAnswer {
			return msg, nil
		}
	}

	parseErr := s.agent.handleContentToolCalls(&msg)
	if parseErr == nil || parseErr.XMLFound {
		return msg, nil
	}

	return msg, nil
}

// sanitizeHistoryForRetry removes the malformed submit_final_answer tool call
// (and its matching tool result) from history so the retry request does not
// contain truncated JSON that the server would reject.
func sanitizeHistoryForRetry(history []proxy.Message) []proxy.Message {
	removeIDs := make(map[string]bool)
	var sanitized []proxy.Message
	for _, m := range history {
		if m.Role == proxy.AssistantRole && len(m.ToolCalls) > 0 {
			var filtered []proxy.ToolCall
			for _, tc := range m.ToolCalls {
				if tc.Function.Name == models.ToolSubmitFinalAnswer {
					removeIDs[tc.ID] = true
				} else {
					filtered = append(filtered, tc)
				}
			}
			if len(filtered) > 0 || m.Content != "" {
				m.ToolCalls = filtered
				sanitized = append(sanitized, m)
			}
		} else if m.Role == proxy.ToolRole && removeIDs[m.ToolCallID] {
			continue
		} else {
			sanitized = append(sanitized, m)
		}
	}
	return sanitized
}

// cacheToolResult stores the result of a successful terminal command in the
// LRU cache so subsequent identical calls can be pre-empted.
func (s *runSession) cacheToolResult(tc proxy.ToolCall, result any) {
	cache := s.agent.toolCache
	if cache == nil {
		return
	}

	resultStr, ok := result.(string)
	if !ok || resultStr == "" || isToolErrorResult(resultStr) {
		return
	}

	cache.Add(tc.Function.Arguments, resultStr)
}

// executeTurn runs one LLM call, parses tool calls from content, validates
// them, and deduplicates.  A non-nil parseErr means the model produced
// malformed XML/native tool calls — the caller decides whether to escalate.
func (a *Agent) executeTurn(ctx context.Context, history *[]proxy.Message) (proxy.Message, *proxy.ParseError, []proxy.Tool, error) {
	turnCtx, turnCancel := context.WithTimeout(ctx, AgentTurnTimeout)
	defer turnCancel()

	toolsList, err := a.provider.ListTools(turnCtx)
	if err != nil {
		return proxy.Message{}, nil, nil, fmt.Errorf("failed to list tools: %w", err)
	}

	*history = a.applyPhysicalSieve(*history)

	// Inject execution state at index [1] so it survives sieve truncation
	// and keeps the model aware of completed steps.  Only for automation
	// runs with an active state object.
	if a.state != nil && len(*history) >= 1 {
		stateMsg := proxy.Message{Role: proxy.SystemRole, Content: a.state.ToCompactState()}
		inserted := false
		for i, m := range *history {
			if m.Role == proxy.SystemRole && strings.HasPrefix(m.Content, "Goal: ") {
				// Update existing state block in-place
				(*history)[i] = stateMsg
				inserted = true
				a.logger.Debug("state block updated in-place", "index", i, "content", a.state.ToCompactState())
				break
			}
		}
		if !inserted {
			// Inject at index [1], right after the base system prompt
			*history = append((*history)[:1], append([]proxy.Message{stateMsg}, (*history)[1:]...)...)
			a.logger.Debug("state block injected at index 1", "history_len", len(*history), "content", a.state.ToCompactState())
		}
	}

	msg, err := a.computeNextResponse(turnCtx, *history, toolsList)
	if err != nil {
		return proxy.Message{}, nil, nil, err
	}

	a.logger.Debug("raw model response",
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
		a.logger.Warn("tool call parse error",
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

	if s.isAutomation {
		if s.agent.isPrematureTermination(turnMsg, s.history) {
			s.agent.logger.Warn("automation task — premature termination detected", "step", s.steps)
			return turnMsg.Content, true, nil
		}

		if parseErr != nil {
			errKind := parseErrorKind(parseErr)
			if errKind == s.lastParseErrorKind {
				s.parseErrorStreak++
			} else {
				s.parseErrorStreak = 0
				s.lastParseErrorKind = errKind
			}
			s.totalErrorStreak++
			if s.totalErrorStreak >= sessionModelCompatNotifyAfter && !s.modelCompatNotified {
				s.modelCompatNotified = true
				s.agent.notifyModelCompatWarning(s.agent.useNativeTools)
			}

			availableNames := proxy.AvailableToolNames(toolsList)
			feedback := parseErr.Feedback(availableNames)

			if s.parseErrorStreak >= sessionParseErrorEscalation {
				feedback = fmt.Sprintf(prompts.ParseErrorEscalationPrefix, feedback)
			}

			s.agent.logger.Debug("injecting specific parse-error feedback",
				"error", parseErr.Error(),
				"streak", s.parseErrorStreak,
				"feedback", feedback,
			)
			s.history = append(s.history, proxy.Message{
				Role:    proxy.UserRole,
				Content: feedback,
			})
		} else {
			s.agent.logger.Warn("turn resulted in no action - nagging model", "step", s.steps, "nag", prompts.AutomationNagPrompt)
			s.history = append(s.history, proxy.Message{
				Role:    proxy.UserRole,
				Content: prompts.AutomationNagPrompt,
			})
		}
		return "", false, nil
	}

	if s.agent.isPrematureTermination(turnMsg, s.history) {
		s.agent.logger.Info("premature termination detected — model is repeating or producing empty output")
		return turnMsg.Content, true, nil
	}
	// Non-automation: first response is always text-only (model introduces itself)
	if s.steps == 1 {
		return turnMsg.Content, true, nil
	}
	if s.agent.countConsecutiveChat(s.history) >= sessionConsecutiveChatExit {
		return turnMsg.Content, true, nil
	}
	if s.agent.precededByToolResult(s.history) {
		return turnMsg.Content, true, nil
	}

	return "", false, nil
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
		a.logger.Debug("xml tool calls parsed from content", "count", len(calls))
		return nil
	}

	if a.useNativeTools {
		nativeCleaned, nativeCalls, nativeErr := proxy.ParseNativeToolCalls(msg.Content)
		if nativeErr == nil && len(nativeCalls) > 0 {
			msg.Content = nativeCleaned
			msg.ToolCalls = append(msg.ToolCalls, nativeCalls...)
			a.logger.Debug("native format tool calls parsed from content", "count", len(nativeCalls))
			return nil
		}
	}

	return parseErr
}

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
	if s.agent.memoryStore == nil || s.memoryFlushSent {
		return
	}

	totalChars := 0
	for _, m := range s.history {
		totalChars += len(m.Content)
	}
	if totalChars == 0 || s.agent.contextBudget == 0 {
		return
	}

	ratio := float64(totalChars) / float64(s.agent.contextBudget)
	if ratio < 0.7 {
		return
	}

	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.PreSieveMemoryNudge,
	})
	s.memoryFlushSent = true
}

// normalizeArgs produces a stable string for failure tracking by sorting
// JSON keys. Non-JSON args are used as-is.
func normalizeArgs(raw string) string {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return raw
	}
	return string(canonical)
}

// isToolErrorResult checks whether a tool result content represents an error.
// Detects JSON {"error": "..."} format (from tool_exec.go error path),
// "Error" prefix strings, and terminal exit_code != 0.
func isToolErrorResult(content string) bool {
	if strings.HasPrefix(content, "Error") {
		return true
	}
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return false
	}
	m, ok := parsed.(map[string]any)
	if !ok {
		return false
	}
	if _, hasError := m["error"]; hasError {
		return true
	}
	if exitCode, hasExitCode := m["exit_code"]; hasExitCode {
		if code, ok := exitCode.(float64); ok && code != 0 {
			return true
		}
	}
	return false
}

// findToolResultByID walks history backwards to find a tool-role message
// matching the given tool_call_id.
func findToolResultByID(history []proxy.Message, callID string) *proxy.Message {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.ToolRole && history[i].ToolCallID == callID {
			return &history[i]
		}
	}
	return nil
}

// applyFailGuidance scans tool results from the latest turn, feeds errors into
// the failTracker, and appends guidance to the tool result content when a tool
// has failed repeatedly.  Guidance is appended inline (Hermes-style) rather
// than as a separate user message.
func (s *runSession) applyFailGuidance(turnMsg proxy.Message) {
	for _, tc := range turnMsg.ToolCalls {
		if tc.Function.Name == models.ToolSubmitFinalAnswer || tc.Function.Name == models.ToolSystemError {
			continue
		}
		result := findToolResultByID(s.history, tc.ID)
		if result == nil {
			continue
		}
		isError := isToolErrorResult(result.Content)
		argsHash := normalizeArgs(tc.Function.Arguments)
		guidance := s.ft.record(tc.Function.Name, argsHash, isError)
		if guidance != "" {
			result.Content += "\n\n" + guidance
		}
	}
}

// findLastToolResultByToolName walks backwards through history to find the
// most recent tool result for a given tool name.
func findLastToolResultByToolName(history []proxy.Message, toolName string) (string, *proxy.Message) {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != proxy.AssistantRole {
			continue
		}
		for _, tc := range history[i].ToolCalls {
			if tc.Function.Name == toolName {
				if result := findToolResultByID(history, tc.ID); result != nil {
					return tc.Function.Arguments, result
				}
			}
		}
	}
	return "", nil
}
