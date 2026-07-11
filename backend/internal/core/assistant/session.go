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
	maxBatchedSubmitRetries       = 3    // consecutive batched submit_final_answer rejections before killing session
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

	// batchedSubmitRetries counts consecutive turns where submit_final_answer
	// was batched with other tools. After maxBatchedSubmitRetries the session
	// is killed to prevent an infinite rejection loop.
	batchedSubmitRetries int

	parseErrorStreak    int
	lastParseErrorKind  string
	totalErrorStreak    int
	modelCompatNotified bool

	rd              repetitionDetector
	memoryFlushSent bool // prevents repeated pre-sieve nudges across turns
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

func (s *runSession) handleToolCallParseError(err error) {
	switch {
	case isJSONSyntaxError(err):
		s.agent.deps.Logger.Warn("server-side tool call JSON parse error (syntax), sending JSON-escaped hint", "error", err)
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

func (s *runSession) checkSubmitFinalAnswer(turnMsg *proxy.Message) (string, bool) {
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

		if s.steps >= s.agent.config.MaxSteps && !s.warnedAdvisory {
			s.warnedAdvisory = true
			s.agent.deps.Logger.Warn("agent exceeded advisory step limit, continuing", "steps", s.steps)
		}

		s.maybeFlushMemoryBeforeTurn()

		s.agent.notifyStepStart(s.steps)
		s.agent.notifyThinking()

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
				s.starvationCount++
				s.handleToolCallParseError(err)
				continue
			}

			return "", s.history, err
		}

		s.sieveStreak = 0

		if len(turnMsg.ToolCalls) > 0 {
			s.resetParseErrorState()

			isDuplicate, nagPrompt, dupErr := s.rd.check(s.agent.deps.Logger, turnMsg.ToolCalls)
			if dupErr != nil {
				return "", s.history, dupErr
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
				continue
			}

			s.trimLargeWriteContent(&turnMsg)

			s.history = append(s.history, turnMsg)
			if err := s.agent.processToolCalls(s.ctx, turnMsg, &s.history); err != nil {
				return "", s.history, err
			}

			// Only exit on solo submit_final_answer. When submit is batched
			// with other tools, processToolCalls already rejected the entire
			// batch and appended error results. The model sees the rejection
			// on the next turn and resubmits properly. Bailing out here would
			// leave the session with error tool results and no final answer.
			submitSolo := len(turnMsg.ToolCalls) == 1
			if submitSolo {
				if content, done := s.checkSubmitFinalAnswer(&turnMsg); done {
					// Sync the extracted summary into the history entry stored
					// earlier by append (value copy).  processToolCalls appended
					// tool results after it, so search backward for the assistant message.
					for i := len(s.history) - 1; i >= 0; i-- {
						if s.history[i].Role == proxy.AssistantRole {
							s.history[i].Content = content
							break
						}
					}
					// Notify AFTER checkSubmitFinalAnswer so the SSE event
					// carries the extracted report content (not empty string).
					// Without this, the live webhook view never gets the
					// final answer text and shows reasoning instead.
					s.agent.notify(EventMessage, turnMsg)
					s.agent.notifyLifecycle("completed", map[string]any{
						"content": content,
					})
					return content, s.history, nil
				}
			} else {
				// submit_final_answer was in a batch — rejected.
				s.batchedSubmitRetries++
				if s.batchedSubmitRetries >= maxBatchedSubmitRetries {
					return "", s.history, fmt.Errorf("agent stalled: submit_final_answer batched with other tools %d times in a row", s.batchedSubmitRetries)
				}
			}
			// Fallthrough notify for non-submit turns: processToolCalls has run,
			// frontend needs tool_call/tool_result events followed by EventMessage.
			// For submit turns this fires inside the done branch above (after
			// checkSubmitFinalAnswer populates turnMsg.Content with the report).
			s.agent.notify(EventMessage, turnMsg)
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
		// If XMLFound is false, the model produced plain text without any
		// tool call attempt. In native-tools mode with tool_choice:required
		// this is a protocol violation — nag to retry instead.
		// Otherwise exit with the text as a conversational reply.
		if !parseErr.XMLFound && strings.TrimSpace(turnMsg.Content) != "" {
			if !s.agent.config.UseNativeTools || len(toolsList) == 0 {
				return turnMsg.Content, true, nil
			}
			parseErr = nil
		}
		if parseErr != nil {
			if len(toolsList) == 0 {
				return "", true, nil
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
			return "", false, nil
		}
	}

	if len(toolsList) == 0 && strings.TrimSpace(turnMsg.Content) != "" {
		return turnMsg.Content, true, nil
	}

	s.agent.deps.Logger.Warn("no tool calls - nagging model", "step", s.steps)
	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.AutomationNagPrompt,
	})
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
	if ratio < 0.7 {
		return
	}

	s.history = append(s.history, proxy.Message{
		Role:    proxy.UserRole,
		Content: prompts.PreSieveMemoryNudge,
	})
	s.memoryFlushSent = true
}
