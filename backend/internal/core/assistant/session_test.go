package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/repetition"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

type testLogger struct {
	T *testing.T
}

func (l *testLogger) Debug(msg string, args ...any)   { l.T.Logf("DEBUG: "+msg, args...) }
func (l *testLogger) Info(msg string, args ...any)    { l.T.Logf("INFO: "+msg, args...) }
func (l *testLogger) Warn(msg string, args ...any)    { l.T.Logf("WARN: "+msg, args...) }
func (l *testLogger) Error(msg string, args ...any)   { l.T.Logf("ERROR: "+msg, args...) }
func (l *testLogger) With(args ...any) logging.Logger { return l }
func (l *testLogger) SetLevel(logging.Level)          {}
func (l *testLogger) Level() logging.Level            { return logging.LevelDebug }

var _ logging.Logger = (*testLogger)(nil)

func TestHasToolCallMarker(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "empty string",
			content: "",
			want:    false,
		},
		{
			name:    "normal text no markers",
			content: "The temperature is 22 degrees Celsius.",
			want:    false,
		},
		{
			name:    "full tool_call tag",
			content: "Now let me check.\n<tool_call>\n{\"tool\":\"read_file\"}\n</tool_call>",
			want:    true,
		},
		{
			name:    "truncated tool_call — no closing tag",
			content: "Let me get more data.\n<tool_call>{\"tool\":\"get_more\"",
			want:    true,
		},
		{
			name:    "full function tag",
			content: "text <function name=\"x\"><parameter name=\"y\">z</parameter></function> text",
			want:    true,
		},
		{
			name:    "truncated function — no closing tag",
			content: "Check <function name=\"read_file\"><parameter name=\"path\">test.txt",
			want:    true,
		},
		{
			name:    "tool tag with space",
			content: "<tool name=\"x\">...</tool>",
			want:    true,
		},
		{
			name:    "uppercase TOOL_CALL truncated",
			content: "text <TOOL_CALL>{\"tool\":\"x\"}",
			want:    true,
		},
		{
			name:    "uppercase TOOL_CALL complete",
			content: "<TOOL_CALL>{\"tool\":\"x\"}</TOOL_CALL>",
			want:    true,
		},
		{
			name:    "uppercase FUNCTION truncated",
			content: "<FUNCTION name=\"x\"><PARAMETER name=\"y\">z</PARAMETER>",
			want:    true,
		},
		{
			name:    "mixed case tool call",
			content: "text <Tool_Call>{\"tool\":\"x\"}",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasToolCallMarker(tt.content)
			if got != tt.want {
				t.Errorf("hasToolCallMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripThinkBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty string", content: "", want: ""},
		{name: "plain text unchanged", content: "Hello world", want: "Hello world"},
		{name: "think block only", content: "<think>reasoning here</think>", want: ""},
		{name: "think block + visible text", content: "<think>reasoning</think>Final answer", want: "Final answer"},
		{name: "think block with attributes", content: "<think depth=\"3\">hmm</think>done", want: "done"},
		{name: "multiple think blocks", content: "<think>first</think>middle<think>second</think>end", want: "middleend"},
		{name: "reasoning block", content: "<reasoning>deep thought</reasoning>answer", want: "answer"},
		{name: "REASONING_SCRATCHPAD uppercase", content: "<REASONING_SCRATCHPAD>plan</REASONING_SCRATCHPAD>result", want: "result"},
		{name: "mixed tag types", content: "<think>hmm</think><reasoning>why</reasoning>final", want: "final"},
		{name: "unbalanced — no closing tag", content: "<think>truncated reasoning", want: "<think>truncated reasoning"},
		{name: "visible text before think block", content: "Start.<think>reasoning</think>", want: "Start."},
		{name: "think block in middle of text", content: "A<think>reason</think>B", want: "AB"},
		{name: "whitespace after stripping", content: "<think>r</think>   ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkBlocks(tt.content)
			if got != tt.want {
				t.Errorf("stripThinkBlocks() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleNoToolCalls(t *testing.T) {
	toolResultHistory := []proxy.Message{
		{Role: proxy.ToolRole, Content: "data"},
	}
	tests := []struct {
		name                 string
		content              string
		reasoningContent     string
		history              []proxy.Message
		toolsList            []proxy.Tool
		lastContentWithTools string
		postToolNudgeCount   int
		finalizeAttempts     int
		wantDone             bool
		wantReplyContains    string
		wantNudged           bool // postToolNudgeCount incremented / nag injected
		wantFinalized        bool // finalizeAttempts incremented / finalize injected
	}{
		{
			name:              "native tools text accepted",
			content:           "# Final Report\nTask completed successfully.",
			toolsList:         nil,
			wantDone:          true,
			wantReplyContains: "Task completed successfully",
		},
		{
			name:                 "content-with-tools fallback",
			content:              "",
			reasoningContent:     "thinking",
			lastContentWithTools: "Previous saved answer",
			toolsList:            []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "ping"}}},
			wantDone:             true,
			wantReplyContains:    "Previous saved answer",
		},
		{
			name:             "re-arm nudge after tool result",
			content:          "",
			reasoningContent: "thinking",
			history:          toolResultHistory,
			toolsList:        []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "ping"}}},
			wantDone:         false,
			wantNudged:       true,
		},
		{
			name:               "finalize once nudges exhausted",
			content:            "",
			reasoningContent:   "thinking",
			history:            toolResultHistory,
			toolsList:          []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "ping"}}},
			postToolNudgeCount: postToolNudgeMax,
			wantDone:           true,
			wantReplyContains:  "Final report: task done",
			wantFinalized:      true,
		},
		{
			name:               "terminal when ladder exhausted",
			content:            "",
			reasoningContent:   "thinking",
			history:            toolResultHistory,
			toolsList:          []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "ping"}}},
			postToolNudgeCount: postToolNudgeMax,
			finalizeAttempts:   1,
			wantDone:           true,
			wantReplyContains:  "",
		},
	}

	agent := NewAgent(&MockClient{
		StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, errors.New("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			// Finalization turn (ToolChoice=none) returns a real report.
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    proxy.AssistantRole,
				Content: "Final report: task done.",
			}}}}, nil
		},
	}, &MockProvider{}, &MockEngine{}, AgentOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newRunSession(agent, context.Background(), tt.history)
			// Mirror production: run() wires the runS back-pointer before any
			// turn runs; executeTurn's tools-disabled flag consumption depends
			// on it.
			agent.runS = s
			s.prompt.lastContentWithTools = tt.lastContentWithTools
			s.recovery.postToolNudgeCount = tt.postToolNudgeCount
			s.finalize.finalizeAttempts = tt.finalizeAttempts

			turnMsg := proxy.Message{
				Role:             proxy.AssistantRole,
				Content:          tt.content,
				ReasoningContent: tt.reasoningContent,
			}

			reply, done, err := s.handleNoToolCalls(turnMsg, nil, tt.toolsList)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			if !strings.Contains(reply, tt.wantReplyContains) {
				t.Errorf("reply = %q, expected to contain %q", reply, tt.wantReplyContains)
			}
			if tt.wantNudged {
				if s.recovery.postToolNudgeCount != tt.postToolNudgeCount+1 {
					t.Errorf("postToolNudgeCount = %d, want %d", s.recovery.postToolNudgeCount, tt.postToolNudgeCount+1)
				}
				last := s.history[len(s.history)-1]
				if last.Role != proxy.UserRole || !strings.Contains(last.Content, "SYSTEM: Continue") {
					t.Errorf("expected nag prompt as last history entry, got role=%v content=%q", last.Role, last.Content[:min(len(last.Content), 40)])
				}
			}
			if tt.wantFinalized {
				if s.finalize.finalizeAttempts != tt.finalizeAttempts+1 {
					t.Errorf("finalizeAttempts = %d, want %d", s.finalize.finalizeAttempts, tt.finalizeAttempts+1)
				}
				if s.finalize.textOnlyNextTurn {
					t.Error("textOnlyNextTurn should be consumed by the finalization turn")
				}
				if n := len(s.history); n < 2 {
					t.Fatalf("history too short after finalization: %d", n)
				}
				finalizePrompt := s.history[len(s.history)-2]
				if finalizePrompt.Role != proxy.UserRole || !strings.Contains(finalizePrompt.Content, "FINAL REPORT") {
					t.Errorf("expected finalize prompt before the finalization turn, got role=%v content=%q", finalizePrompt.Role, finalizePrompt.Content[:min(len(finalizePrompt.Content), 40)])
				}
			}
		})
	}
}

// TestFlagSplit_NagDoesNotDisableHardCap verifies the post-tool nudge counter
// does not disable the forced-completion hard cap at MaxSteps*2.
func TestFlagSplit_NagDoesNotDisableHardCap(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5})
	s := newRunSession(agent, context.Background(), nil)
	s.recovery.postToolNudgeCount = postToolNudgeMax // nudges already exhausted
	s.steps = agent.config.MaxSteps * 2              // at the hard-cap threshold

	stopped, _, _ := s.checkForcedCompletion()
	if !stopped {
		t.Fatal("hard cap should fire despite exhausted nudges")
	}
	if !s.hardCapTriggered {
		t.Error("hardCapTriggered should be set after the hard cap fires")
	}
}

// TestFlagSplit_HardCapIndependent verifies the hard cap is governed solely by
// its own flag and does not reset across calls.
func TestFlagSplit_HardCapIndependent(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5})
	s := newRunSession(agent, context.Background(), nil)
	s.steps = agent.config.MaxSteps * 2

	if stopped, _, _ := s.checkForcedCompletion(); !stopped {
		t.Fatal("expected hard cap to fire on first call at threshold")
	}
	// Second call must be a no-op (already triggered), not a re-fire.
	if stopped, _, _ := s.checkForcedCompletion(); stopped {
		t.Error("hard cap should not re-fire once hardCapTriggered is set")
	}
	// The recovery reset must never clear the irreversible cap: clearing it
	// would re-arm forced completion past MaxSteps*2.
	s.resetParseErrorState()
	if !s.hardCapTriggered {
		t.Error("resetParseErrorState must not clear hardCapTriggered")
	}
	if stopped, _, _ := s.checkForcedCompletion(); stopped {
		t.Error("hard cap should stay latched after the recovery reset")
	}
}

// A playbook that saves before it answers makes the model write its report in
// the same turn as memory_update; the next turn is a short sign-off. Driven
// through the real turn handlers: handleToolTurn must save the report beside a
// housekeeping-only call, handleTextTurn must keep it over the sign-off, and a
// later work turn must drop the saved text (it is stale narration by then).
func TestHandleToolTurn_ReportBesideMemoryUpdateSurvivesSignOff(t *testing.T) {
	report := "# Brief\n" + strings.Repeat("| item | detail |\n", 20)
	signOff := proxy.Message{Role: proxy.AssistantRole, Content: "Saved to memory. Done, see above.", FinishReason: "stop"}
	saveTurn := proxy.Message{Role: proxy.AssistantRole, Content: report, ToolCalls: []proxy.ToolCall{
		{ID: "s1", Type: "function", Function: proxy.FunctionCall{Name: models.ToolMemoryUpdate, Arguments: `{"content":"brief saved"}`}},
	}}
	workTurn := proxy.Message{Role: proxy.AssistantRole, Content: "Checking one more source.", ToolCalls: []proxy.ToolCall{
		{ID: "w1", Type: "function", Function: proxy.FunctionCall{Name: models.ToolMemorySearch, Arguments: `{"query":"more"}`}},
	}}
	journalTurn := proxy.Message{Role: proxy.AssistantRole, Content: "Recording this run in the journal now.", ToolCalls: []proxy.ToolCall{
		{ID: "j1", Type: "function", Function: proxy.FunctionCall{Name: models.ToolAutomationJournal, Arguments: `{"entry":"brief done"}`}},
	}}
	toolsList := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolMemoryUpdate}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolMemorySearch}},
		{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolAutomationJournal}},
	}

	tests := []struct {
		name      string
		toolTurns []proxy.Message
		want      string
	}{
		{"report beside memory_update survives the sign-off", []proxy.Message{saveTurn}, report},
		{"a later work turn drops the saved text", []proxy.Message{saveTurn, workTurn}, signOff.Content},
		{"a later housekeeping narration does not replace the report", []proxy.Message{saveTurn, journalTurn}, report},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &MockEngine{Result: "ok"}
			agent := NewAgent(&MockClient{}, &MockProvider{Tools: toolsList}, engine, AgentOptions{})
			s := newRunSession(agent, context.Background(), []proxy.Message{{Role: proxy.UserRole, Content: "write the brief"}})
			agent.runS = s

			for _, turn := range tt.toolTurns {
				if done, _, err := s.handleToolTurn(turn, toolsList); done || err != nil {
					t.Fatalf("tool turn ended the run: done=%v err=%v", done, err)
				}
			}
			if engine.Calls != len(tt.toolTurns) {
				t.Fatalf("engine calls = %d, want %d (every tool turn must execute)", engine.Calls, len(tt.toolTurns))
			}
			done, reply, err := s.handleTextTurn(signOff, nil, toolsList)
			if err != nil || !done {
				t.Fatalf("expected natural completion, got done=%v err=%v", done, err)
			}
			if strings.TrimSpace(reply) != strings.TrimSpace(tt.want) {
				t.Errorf("reply = %q, want %q", clip(reply, 60), clip(tt.want, 60))
			}
		})
	}
}

// The nudges say what the profile wants: a tool-first run is told to call a tool, an answer-first one to answer.
func TestNudgesFollowTheProfile(t *testing.T) {
	newSession := func(answerFirst bool) *runSession {
		agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{Profile: profile.Profile{ID: "p", AnswerFirst: answerFirst}})
		return newRunSession(agent, context.Background(), nil)
	}
	toolFirst, answerFirst := newSession(false), newSession(true)

	if got := toolFirst.emptyTurnNag(); got != prompts.AutomationNagPrompt {
		t.Errorf("tool-first empty-turn nag = %q", got)
	}
	if got := answerFirst.emptyTurnNag(); got != prompts.AnswerFirstNagPrompt {
		t.Errorf("answer-first empty-turn nag = %q", got)
	}
	if toolFirst.stuckNag(false) != prompts.ReasoningStuckNag || toolFirst.stuckNag(true) != prompts.ReasoningStuckEscalatedNag {
		t.Error("tool-first stuck nags changed")
	}
	if answerFirst.stuckNag(false) != prompts.AnswerFirstStuckNag || answerFirst.stuckNag(true) != prompts.AnswerFirstStuckEscalatedNag {
		t.Error("answer-first stuck nags are the answer-first ones")
	}
	for _, nag := range []string{prompts.AnswerFirstNagPrompt, prompts.AnswerFirstStuckNag, prompts.AnswerFirstStuckEscalatedNag} {
		if strings.Contains(strings.ToLower(nag), "only a tool call") || strings.Contains(strings.ToLower(nag), "call a tool immediately") {
			t.Errorf("an answer-first nudge must not demand a tool call: %q", nag)
		}
	}
}

// repeatingModel calls the same tool with the same arguments on every turn that offers tools, and answers in plain
// text on a turn that does not (the finalization turn).
func repeatingModel(answer string) *MockClient {
	return &MockClient{StreamFunc: func(_ context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		ch := make(chan *proxy.ChatResponse, 1)
		if req.ToolChoice == proxy.ToolChoiceNone || len(req.Tools) == 0 {
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: answer}}}}
		} else {
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
				ID: "c", Type: "function", Function: proxy.FunctionCall{Name: "probe_tool", Arguments: `{"url":"https://example.test/x"}`},
			}}}}}}
		}
		close(ch)
		return ch, nil
	}}
}

func runRepeating(t *testing.T, channel EventChannel, answer string) (string, []proxy.Message, error) {
	t.Helper()
	provider := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "probe_tool"}}}}
	agent := NewAgent(repeatingModel(answer), provider, &MockEngine{Result: "partial data"}, AgentOptions{MaxSteps: 20, Channel: channel, GlobalTimeout: 10 * time.Second})
	return agent.Execute(context.Background(), []proxy.Message{{Role: proxy.SystemRole, Content: "sys"}, {Role: proxy.UserRole, Content: "look it up"}})
}

// When a chat run is stopped for repeating itself, the user gets an answer from what the run already found — a
// tools-off turn — instead of an error and nothing, as the 2026-10-18 local run did after a 503.
func TestRun_ChatLoopStopAnswersFromWhatItFound(t *testing.T) {
	const answer = "Here is what I found: it is sunny and 8°C. I could not get tomorrow's forecast."
	reply, history, err := runRepeating(t, ChannelAssistant, answer)
	if err != nil {
		t.Fatalf("a loop-stopped chat run must answer, got error: %v", err)
	}
	if !strings.Contains(reply, "sunny and 8°C") {
		t.Errorf("reply = %q, want the finalization answer", reply)
	}
	if countContent(history, prompts.LoopStopFinalizePrompt) != 1 {
		t.Error("the finalization instruction must be in the history once, as a control message")
	}
	if !isAgentControlMessage(proxy.Message{Role: proxy.UserRole, Content: prompts.LoopStopFinalizePrompt}) {
		t.Error("the finalization instruction must be a control message, so it is never mistaken for user text")
	}
}

// An automation has nobody reading it live: a loop is a failure it must report, not paper over with an answer.
func TestRun_AutomationLoopStopStillFails(t *testing.T) {
	_, _, err := runRepeating(t, ChannelAutomation, "an answer")
	var loopErr *repetition.LoopError
	if !errors.As(err, &loopErr) {
		t.Fatalf("an automation stopped for looping must keep its LoopError, got %v", err)
	}
}

// With no tool work to answer from there is nothing honest to say: the loop error stands.
func TestStopOnLoop_WithoutToolWorkKeepsTheError(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5, Channel: ChannelAssistant})
	s := newRunSession(agent, context.Background(), []proxy.Message{{Role: proxy.SystemRole, Content: "sys"}, {Role: proxy.UserRole, Content: "hi"}})
	loopErr := repetition.NewLoopError("loop")
	done, reply, err := s.stopOnLoop(loopErr)
	if !done || reply != "" || !errors.Is(err, loopErr) {
		t.Errorf("stopOnLoop = %v, %q, %v; want done with the original error", done, reply, err)
	}
}

// A model that keeps calling tools even when they are off gives the finalization nothing to show: the loop error
// stands rather than a statistics summary dressed up as an answer.
func TestRun_ChatLoopStopKeepsTheErrorWhenTheModelWillNotAnswer(t *testing.T) {
	client := &MockClient{StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		ch := make(chan *proxy.ChatResponse, 1)
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
			ID: "c", Type: "function", Function: proxy.FunctionCall{Name: "probe_tool", Arguments: `{"url":"https://example.test/x"}`},
		}}}}}}
		close(ch)
		return ch, nil
	}}
	provider := &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "probe_tool"}}}}
	agent := NewAgent(client, provider, &MockEngine{Result: "partial data"}, AgentOptions{MaxSteps: 20, GlobalTimeout: 10 * time.Second})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: proxy.SystemRole, Content: "sys"}, {Role: proxy.UserRole, Content: "look it up"}})
	var loopErr *repetition.LoopError
	if !errors.As(err, &loopErr) {
		t.Fatalf("want the LoopError when the model will not answer, got %v", err)
	}
}

// A read_file the model repeats right after a result that said part of the file was NOT SHOWN is not a pointless
// repeat: the model is trying to read the rest and left out the arguments. The nag written for code tasks ("compile
// and run it, do not read the same file again") told such a run that further reads were disabled (seen live, Qwen 16k).
func TestReadFileRepeatNag_PointsAtTheMarkerWhenTheLastResultHadAGap(t *testing.T) {
	gap := proxy.Message{Role: proxy.ToolRole, Content: "\"start of file\\n[NOT SHOWN: bytes 1363-5635, 4272 bytes, 60% of the file. Call read_file with offset=1363 and limit=2800 to read it.]\\nend\""}
	whole := proxy.Message{Role: proxy.ToolRole, Content: "\"a whole small file\""}
	call := proxy.Message{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{ID: "c1", Function: proxy.FunctionCall{Name: models.ToolFileRead}}}}

	if got := readFileRepeatNag([]proxy.Message{call, gap}); got != prompts.ReadFileRepeatedGapNagPrompt {
		t.Errorf("after a gap result: %q", got)
	}
	if got := readFileRepeatNag([]proxy.Message{call, whole}); got != prompts.AutomationReadFileNagPrompt {
		t.Errorf("after a whole-file result the old nag stays: %q", got)
	}
	if got := readFileRepeatNag([]proxy.Message{gap, call, whole}); got != prompts.AutomationReadFileNagPrompt {
		t.Errorf("only the most recent result counts: %q", got)
	}
	if got := readFileRepeatNag(nil); got != prompts.AutomationReadFileNagPrompt {
		t.Errorf("no history: %q", got)
	}
}

func TestReadFileRepeatedGapNag_IsAControlMessageAndTellsTheModelWhatToDo(t *testing.T) {
	if !isAgentControlMessage(proxy.Message{Role: proxy.UserRole, Content: prompts.ReadFileRepeatedGapNagPrompt}) {
		t.Error("the nag must be hidden from the conversation like the other control prompts")
	}
	n := prompts.ReadFileRepeatedGapNagPrompt
	if !strings.HasPrefix(n, "SYSTEM:") || !strings.Contains(n, "offset") || !strings.Contains(n, "NOT SHOWN") {
		t.Errorf("nag: %q", n)
	}
	if strings.ContainsAny(n, "<>&") {
		t.Error("a nag the model reads must hold no characters that are escaped when encoded")
	}
	if !strings.Contains(prompts.FileGapMarker, prompts.FileGapMarkerLead) {
		t.Error("the helper detects the marker by its lead, which must be what the marker starts with")
	}
}
