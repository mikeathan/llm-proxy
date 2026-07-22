package assistant

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/core/proxy"
)

func TestContainsSubstantiveToolCall(t *testing.T) {
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
			name:    "no tool call tag",
			content: "plain text without any tags",
			want:    false,
		},
		{
			name:    "empty tool call block",
			content: "some text <tool_call></tool_call> more text",
			want:    false,
		},
		{
			name:    "whitespace-only body",
			content: "planning <tool_call>  \n  </tool_call> text",
			want:    false,
		},
		{
			name:    "empty block then valid block",
			content: "<tool_call></tool_call><tool_call>\n{\"tool\":\"read_file\",\"args\":{\"path\":\"test\"}}\n</tool_call>",
			want:    true,
		},
		{
			name:    "valid JSON tool call",
			content: "text <tool_call>\n{\"tool\":\"read_file\",\"args\":{\"path\":\"config.json\"}}\n</tool_call> text",
			want:    true,
		},
		{
			name:    "valid native format tool call",
			content: "text <tool_call><function=execute_terminal_command><parameter=command>ls</parameter></function></tool_call> text",
			want:    true,
		},
		{
			name:    "dangling opening tag no close",
			content: "text <tool_call>not closed yet",
			want:    false,
		},
		{
			name:    "dangling after empty close",
			content: "<tool_call></tool_call> then <tool_call>{\"tool\"",
			want:    false,
		},
		{
			name:    "malformed body but non-empty",
			content: "<tool_call>abc</tool_call>",
			want:    true,
		},
		{
			name:    "uppercase TOOL_CALL with body",
			content: "<TOOL_CALL>{\"tool\":\"x\"}</TOOL_CALL>",
			want:    true,
		},
		{
			name:    "uppercase TOOL_CALL empty",
			content: "<TOOL_CALL></TOOL_CALL>",
			want:    false,
		},
		{
			name:    "uppercase TOOL_CALL whitespace only",
			content: "<TOOL_CALL>  \n  </TOOL_CALL>",
			want:    false,
		},
		{
			name:    "mixed case Tool_Call with body",
			content: "<Tool_Call>{\"tool\":\"x\"}</Tool_Call>",
			want:    true,
		},
		{
			name:    "uppercase dangling no close",
			content: "<TOOL_CALL>{\"tool\":\"x\"}",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsSubstantiveToolCall(tt.content)
			if got != tt.want {
				t.Errorf("containsSubstantiveToolCall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountEmptyClosedToolCalls(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty string", "", 0},
		{"no tags", "plain reasoning", 0},
		{"one empty", "plan <tool_call></tool_call> more", 1},
		{"three empties", "<tool_call></tool_call>\n<tool_call></tool_call>\n<tool_call></tool_call>", 3},
		{"whitespace body counts", "<tool_call>  \n  </tool_call><tool_call></tool_call>", 2},
		{"substantive not counted", "<tool_call>{\"tool\":\"x\"}</tool_call>", 0},
		{"mixed empty then substantive", "<tool_call></tool_call><tool_call>{\"tool\":\"x\"}</tool_call><tool_call></tool_call>", 2},
		{"dangling open not counted", "<tool_call></tool_call><tool_call>still forming", 1},
		{"uppercase empty", "<TOOL_CALL></TOOL_CALL><TOOL_CALL></TOOL_CALL><TOOL_CALL></TOOL_CALL>", 3},
		{"qwen spiral sample", "Task done.\n\n<tool_call>\n\n</tool_call>\n\n<tool_call>\n\n</tool_call>\n\n<tool_call>\n\n</tool_call>\n\n<tool_call>\n\n</tool_call>", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countEmptyClosedToolCalls(tt.content)
			if got != tt.want {
				t.Errorf("countEmptyClosedToolCalls() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCheckStreamStuck_EmptyToolCallSpiral(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			MaxTokens:       2730,
			ReasoningBudget: 0,
			SkipStuckCheck:  false,
		},
	}

	// Below limit: two empty closed tags — keep streaming (may still form a real call).
	two := &proxy.Message{
		ReasoningContent: "thinking\n<tool_call></tool_call>\n<tool_call></tool_call>\n",
	}
	if agent.checkStreamStuck(two) {
		t.Error("2 empty tool_calls should not trigger spiral stuck")
	}

	// At limit: three closed empties → stuck (same recovery path as char threshold).
	three := &proxy.Message{
		ReasoningContent: "Task complete.\n<tool_call></tool_call>\n<tool_call></tool_call>\n<tool_call></tool_call>\n",
	}
	if !agent.checkStreamStuck(three) {
		t.Error("3 empty tool_calls should trigger spiral stuck")
	}

	// Substantive call present → extract path owns it; spiral must not fire
	// when content would be non-empty after extract. Here pure reasoning with
	// a real body: still spiral-count empties only; one empty + one real = 1 empty.
	mixed := &proxy.Message{
		ReasoningContent: "<tool_call></tool_call><tool_call>{\"tool\":\"read_file\",\"args\":{}}</tool_call>",
	}
	if agent.checkStreamStuck(mixed) {
		t.Error("substantive tool_call present should not spiral-stuck (empty count=1)")
	}

	// Content or native tool calls → never stuck (guards first).
	withContent := &proxy.Message{
		Content:          "final report",
		ReasoningContent: strings.Repeat("<tool_call></tool_call>\n", 10),
	}
	if agent.checkStreamStuck(withContent) {
		t.Error("has content → not stuck")
	}
	withTools := &proxy.Message{
		ToolCalls:        []proxy.ToolCall{{}},
		ReasoningContent: strings.Repeat("<tool_call></tool_call>\n", 10),
	}
	if agent.checkStreamStuck(withTools) {
		t.Error("has tool calls → not stuck")
	}

	// SkipStuckCheck disables spiral too.
	agent.config.SkipStuckCheck = true
	if agent.checkStreamStuck(three) {
		t.Error("SkipStuckCheck should disable empty-tool spiral")
	}
}

func TestTryExtractToolCallFromReasoning(t *testing.T) {
	agent := &Agent{}

	t.Run("no reasoning content", func(t *testing.T) {
		msg := &proxy.Message{ReasoningContent: ""}
		if agent.tryExtractToolCallFromReasoning(msg) {
			t.Error("empty reasoning → false")
		}
	})

	t.Run("no tool_call in reasoning", func(t *testing.T) {
		msg := &proxy.Message{ReasoningContent: "<think>just thinking</think>"}
		if agent.tryExtractToolCallFromReasoning(msg) {
			t.Error("no tool_call pattern → false")
		}
	})

	t.Run("extracts tool call, content stays empty", func(t *testing.T) {
		msg := &proxy.Message{
			Content:          "",
			ReasoningContent: `<think>I should scan the network now.\n<tool_call>{"tool":"scan_local_network","args":{"mode":"fast"}}</tool_call>\n</think>`,
		}
		if !agent.tryExtractToolCallFromReasoning(msg) {
			t.Fatal("should have extracted tool call from reasoning")
		}
		if msg.Content != "" {
			t.Errorf("Content should stay empty after extraction, got %q", msg.Content)
		}
		if len(msg.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
		}
		if msg.ToolCalls[0].Function.Name != "scan_local_network" {
			t.Errorf("expected scan_local_network, got %q", msg.ToolCalls[0].Function.Name)
		}
	})

	t.Run("skips when content already has text", func(t *testing.T) {
		msg := &proxy.Message{
			Content:          "I have completed both phases.",
			ReasoningContent: `<think>I should write the report.\n<tool_call>{"tool":"write_file","args":{"path":"report.md","content":"data"}}</tool_call>\n</think>`,
		}
		if agent.tryExtractToolCallFromReasoning(msg) {
			t.Error("should NOT extract when Content already has visible text")
		}
	})

	t.Run("empty tool_call block is skipped", func(t *testing.T) {
		msg := &proxy.Message{
			Content:          "",
			ReasoningContent: "<tool_call></tool_call>",
		}
		if agent.tryExtractToolCallFromReasoning(msg) {
			t.Error("empty tool_call block should be skipped")
		}
	})
}

// TestNoToolCap_Relaxation covers §2.1 of the automation renderer plan: the
// stream-layer no-tool content cap must NOT amputate a legitimate final answer
// when real work already happened (prior tool result) or no tools are
// configured. It must still terminate the runaway tool-free joke-loop when
// there is no prior tool result and tools are available.
func TestNoToolCap_Relaxation(t *testing.T) {
	const maxTokens = 100

	newAgent := func(useNative bool) *Agent {
		return &Agent{
			config: AgentConfig{
				UseNativeTools: useNative,
				MaxTokens:      maxTokens,
				Channel:        "test",
			},
			deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()},
		}
	}

	// feedStream emits content in 11-char chunks until totalLen is reached.
	feedStream := func(totalLen int) <-chan *proxy.ChatResponse {
		ch := make(chan *proxy.ChatResponse, 1)
		go func() {
			defer close(ch)
			sent := 0
			for sent < totalLen {
				chunk := "xxxxxxxxxxx"
				if remaining := totalLen - sent; remaining < len(chunk) {
					chunk = strings.Repeat("x", remaining)
				}
				ch <- &proxy.ChatResponse{
					Choices: []proxy.Choice{{Delta: proxy.Message{Content: chunk}}},
				}
				sent += len(chunk)
			}
		}()
		return ch
	}

	tests := []struct {
		name            string
		useNative       bool
		priorToolResult bool
		toolsAvailable  bool
		streamLen       int // must exceed maxTokens to trigger the cap path
		wantTerminate   bool
	}{
		{
			name:            "no native tools disables cap entirely",
			useNative:       false,
			priorToolResult: false,
			toolsAvailable:  true,
			streamLen:       maxTokens + 200,
			wantTerminate:   false,
		},
		{
			name:            "native, no prior work, tools available → terminate (runaway loop)",
			useNative:       true,
			priorToolResult: false,
			toolsAvailable:  true,
			streamLen:       maxTokens + 200,
			wantTerminate:   true,
		},
		{
			name:            "native, prior tool result → keep full answer",
			useNative:       true,
			priorToolResult: true,
			toolsAvailable:  true,
			streamLen:       maxTokens + 200,
			wantTerminate:   false,
		},
		{
			name:            "native, no tools available → keep full answer",
			useNative:       true,
			priorToolResult: false,
			toolsAvailable:  false,
			streamLen:       maxTokens + 200,
			wantTerminate:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := newAgent(tt.useNative)
			var fullMsg proxy.Message
			fullMsg.Role = proxy.AssistantRole
			err := agent.processStream(context.Background(), feedStream(tt.streamLen), &fullMsg, tt.priorToolResult, tt.toolsAvailable)
			if err != nil {
				t.Fatalf("processStream returned unexpected error: %v", err)
			}
			terminated := len(fullMsg.Content) < tt.streamLen/2
			if terminated != tt.wantTerminate {
				t.Errorf("terminated=%v, want %v (content len=%d, streamLen=%d)", terminated, tt.wantTerminate, len(fullMsg.Content), tt.streamLen)
			}
			if !tt.wantTerminate && len(fullMsg.Content) < tt.streamLen {
				t.Errorf("legitimate answer was truncated: got %d chars, expected %d", len(fullMsg.Content), tt.streamLen)
			}
		})
	}
}

func TestCleanReasoningContent_Precompiled(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<think>planning</think>hello", "planninghello"},
		{"</think>hello<think>", "hello"},
		{"hello", "hello"},
		{"<think>nested<think>tags</think></think>", "nestedtags"},
		{"  \t <think>x</think> world  ", "x world"},
		{"", ""},
	}
	for _, tt := range tests {
		got := cleanReasoningContent(tt.input)
		if got != tt.expected {
			t.Errorf("cleanReasoningContent(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}

	if thinkTagsRe == nil {
		t.Error("thinkTagsRe must be precompiled, not nil")
	}
}
