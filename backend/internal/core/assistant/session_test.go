package assistant

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
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
			wantDone:           false,
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
			return nil, context.Canceled
		},
	}, &MockProvider{}, &MockEngine{}, AgentOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newRunSession(agent, context.Background(), tt.history)
			s.lastContentWithTools = tt.lastContentWithTools
			s.postToolNudgeCount = tt.postToolNudgeCount
			s.finalizeAttempts = tt.finalizeAttempts

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
				if s.postToolNudgeCount != tt.postToolNudgeCount+1 {
					t.Errorf("postToolNudgeCount = %d, want %d", s.postToolNudgeCount, tt.postToolNudgeCount+1)
				}
				last := s.history[len(s.history)-1]
				if last.Role != proxy.UserRole || !strings.Contains(last.Content, "SYSTEM: Continue") {
					t.Errorf("expected nag prompt as last history entry, got role=%v content=%q", last.Role, last.Content[:min(len(last.Content), 40)])
				}
			}
			if tt.wantFinalized {
				if s.finalizeAttempts != tt.finalizeAttempts+1 {
					t.Errorf("finalizeAttempts = %d, want %d", s.finalizeAttempts, tt.finalizeAttempts+1)
				}
				if !s.textOnlyNextTurn {
					t.Error("textOnlyNextTurn should be set for finalization turn")
				}
				last := s.history[len(s.history)-1]
				if last.Role != proxy.UserRole || !strings.Contains(last.Content, "FINAL REPORT") {
					t.Errorf("expected finalize prompt as last history entry, got role=%v content=%q", last.Role, last.Content[:min(len(last.Content), 40)])
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
	s.postToolNudgeCount = postToolNudgeMax // nudges already exhausted
	s.steps = agent.config.MaxSteps * 2     // at the hard-cap threshold

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
}
