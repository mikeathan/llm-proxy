package assistant

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/reasoning"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// TestApplyRequestConfig covers the shared per-model request-config helper
// (temperature + reasoning wire params) used by both the turn loop
// (buildChatRequest) and plan generation (ExecutionPlanStrategy.Generate).
// TestResolveStreamChunk covers the shared Delta→Message stream-field
// resolution used by both processStream and plan-generation streaming.
func TestResolveStreamChunk(t *testing.T) {
	t.Run("delta wins when present", func(t *testing.T) {
		chunk := resolveStreamChunk(proxy.Choice{
			Delta:   proxy.Message{Content: "delta", ReasoningContent: "dr"},
			Message: proxy.Message{Content: "message", ReasoningContent: "mr"},
		})
		if chunk.Content != "delta" || chunk.ReasoningContent != "dr" {
			t.Errorf("expected delta fields to win, got %+v", chunk)
		}
	})
	t.Run("message fallback when delta empty", func(t *testing.T) {
		chunk := resolveStreamChunk(proxy.Choice{
			Message: proxy.Message{Content: "message", ReasoningContent: "mr", Reasoning: "opaque"},
		})
		if chunk.Content != "message" || chunk.ReasoningContent != "mr" || chunk.Reasoning != "opaque" {
			t.Errorf("expected message fallback, got %+v", chunk)
		}
	})
	t.Run("reasoning details fallback", func(t *testing.T) {
		chunk := resolveStreamChunk(proxy.Choice{
			Message: proxy.Message{ReasoningDetails: []models.ReasoningDetail{{Type: "thinking", Text: "detail"}}},
		})
		if len(chunk.ReasoningDetails) != 1 || chunk.ReasoningDetails[0].Text != "detail" {
			t.Errorf("expected message reasoning_details fallback, got %+v", chunk.ReasoningDetails)
		}
	})
	t.Run("empty choice yields empty chunk", func(t *testing.T) {
		chunk := resolveStreamChunk(proxy.Choice{})
		if chunk.Content != "" || chunk.ReasoningContent != "" || chunk.Reasoning != "" || len(chunk.ReasoningDetails) != 0 || chunk.FinishReason != "" {
			t.Errorf("expected empty chunk, got %+v", chunk)
		}
	})
	t.Run("choice-level finish_reason propagates", func(t *testing.T) {
		// The upstream surfaces finish_reason at the choice level, not inside
		// delta/message — it must reach the resolved chunk for length-truncation
		// detection.
		chunk := resolveStreamChunk(proxy.Choice{
			Delta:        proxy.Message{Content: "partial report"},
			FinishReason: "length",
		})
		if chunk.FinishReason != "length" {
			t.Errorf("expected choice finish_reason to propagate, got %q", chunk.FinishReason)
		}
	})
}

func TestApplyRequestConfig(t *testing.T) {
	newAgent := func(temperature float64) *Agent {
		return &Agent{
			config: AgentConfig{
				Temperature:     temperature,
				ProviderType:    models.ProviderLocal,
				WorkloadClass:   models.WorkloadLocal,
				ReasoningSpec:   reasoning.ReasoningSpec{Mode: reasoning.ModeThinkTokens, Effort: reasoning.EffortMedium, Budget: 512},
				ReasoningBudget: 512,
			},
			deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()},
		}
	}

	t.Run("applies temperature and reasoning wire params", func(t *testing.T) {
		req := proxy.ChatRequest{}
		newAgent(0.7).applyRequestConfig(&req)
		if req.Temperature != 0.7 {
			t.Errorf("expected temperature 0.7, got %v", req.Temperature)
		}
		if req.ThinkingBudgetTokens != 512 {
			t.Errorf("expected thinking_budget_tokens 512, got %d", req.ThinkingBudgetTokens)
		}
	})

	t.Run("zero temperature leaves field unset", func(t *testing.T) {
		req := proxy.ChatRequest{}
		newAgent(0).applyRequestConfig(&req)
		if req.Temperature != 0 {
			t.Errorf("expected temperature 0 (unset), got %v", req.Temperature)
		}
		// Reasoning params still apply regardless of temperature.
		if req.ThinkingBudgetTokens != 512 {
			t.Errorf("expected thinking_budget_tokens 512, got %d", req.ThinkingBudgetTokens)
		}
	})
}

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

func runNotifyCoalescingFeed(t *testing.T, deltas []proxy.Message) (reasoning, content []string, fullMsg proxy.Message) {
	t.Helper()
	agent := &Agent{
		config: AgentConfig{Channel: "test", MaxTokens: 1_000_000},
		deps: AgentRuntimeDeps{
			Logger:   logging.NewNopLogger(),
			Observer: func(ev AgentEvent) {},
		},
	}
	agent.deps.Observer = func(ev AgentEvent) {
		switch ev.Type {
		case EventReasoning:
			reasoning = append(reasoning, ev.Payload.(string))
		case EventToolStream:
			content = append(content, ev.Payload.(string))
		}
	}
	feed := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(feed)
		for _, d := range deltas {
			feed <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: d}}}
		}
	}()
	fullMsg.Role = proxy.AssistantRole
	if err := agent.processStream(context.Background(), feed, &fullMsg, false, false); err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}
	return reasoning, content, fullMsg
}

func assertOrderedSnapshots(t *testing.T, kind string, snapshots []string, want string) {
	t.Helper()
	if len(snapshots) == 0 {
		t.Fatalf("%s: expected at least one snapshot", kind)
	}
	if got := snapshots[len(snapshots)-1]; got != want {
		t.Errorf("%s: final snapshot = %q, want %q", kind, got, want)
	}
	for i := 1; i < len(snapshots); i++ {
		if !strings.HasPrefix(snapshots[i], snapshots[i-1]) {
			t.Fatalf("%s: snapshot %d (%q) is not a prefix of snapshot %d (%q)",
				kind, i-1, snapshots[i-1], i, snapshots[i])
		}
	}
}

func TestProcessStream_NotifyCoalescing(t *testing.T) {
	const chunks = 200
	deltas := make([]proxy.Message, chunks)
	for i := range deltas {
		// Distinct content per chunk so the stream is not repetition-dominated
		// (the repetition guard must not truncate this coalescing test).
		deltas[i] = proxy.Message{Content: fmt.Sprintf("chunk-%04d-%s", i, strings.Repeat("q", 12))}
	}
	_, content, _ := runNotifyCoalescingFeed(t, deltas)
	want := ""
	for _, d := range deltas {
		want += d.Content
	}
	assertOrderedSnapshots(t, "EventToolStream", content, want)
	if len(content) >= chunks {
		t.Errorf("notify was not coalesced: got %d events for %d chunks (want < %d)", len(content), chunks, chunks)
	}
}

func TestProcessStream_NotifyCoalescing_Reasoning(t *testing.T) {
	const chunks = 200
	const chunkLen = 11
	deltas := make([]proxy.Message, chunks)
	for i := range deltas {
		deltas[i] = proxy.Message{ReasoningContent: strings.Repeat("r", chunkLen)}
	}
	reasoning, _, _ := runNotifyCoalescingFeed(t, deltas)
	want := strings.Repeat("r", chunks*chunkLen)
	assertOrderedSnapshots(t, "EventReasoning", reasoning, want)
	if len(reasoning) >= chunks {
		t.Errorf("reasoning notify was not coalesced: got %d events for %d chunks (want < %d)", len(reasoning), chunks, chunks)
	}
}

func TestProcessStream_NotifyCoalescing_ReasoningThenContent(t *testing.T) {
	const perPhase = 100
	deltas := make([]proxy.Message, 0, perPhase*2)
	for i := 0; i < perPhase; i++ {
		deltas = append(deltas, proxy.Message{ReasoningContent: strings.Repeat("r", 11)})
	}
	for i := 0; i < perPhase; i++ {
		// Distinct content per chunk so the stream is not repetition-dominated.
		deltas = append(deltas, proxy.Message{Content: fmt.Sprintf("chunk-%04d-%s", i, strings.Repeat("q", 12))})
	}
	reasoning, content, _ := runNotifyCoalescingFeed(t, deltas)
	wantReasoning := strings.Repeat("r", perPhase*11)
	wantContent := ""
	for _, d := range deltas[perPhase:] {
		wantContent += d.Content
	}
	assertOrderedSnapshots(t, "EventReasoning", reasoning, wantReasoning)
	assertOrderedSnapshots(t, "EventToolStream", content, wantContent)
	if len(reasoning) >= perPhase {
		t.Errorf("reasoning notify was not coalesced: got %d events for %d chunks (want < %d)", len(reasoning), perPhase, perPhase)
	}
	if len(content) >= perPhase {
		t.Errorf("content notify was not coalesced: got %d events for %d chunks (want < %d)", len(content), perPhase, perPhase)
	}
}

func TestProcessStream_NotifyCoalescing_Dedupe(t *testing.T) {
	const stall = 50
	const chunkLen = 11
	deltas := make([]proxy.Message, 0, stall+2)
	for i := 0; i < stall; i++ {
		deltas = append(deltas, proxy.Message{ReasoningContent: strings.Repeat("r", chunkLen)})
	}
	deltas = append(deltas, proxy.Message{ReasoningContent: strings.Repeat("r", chunkLen) + "a"})
	deltas = append(deltas, proxy.Message{ReasoningContent: strings.Repeat("r", chunkLen) + "ab"})
	reasoning, _, _ := runNotifyCoalescingFeed(t, deltas)
	var finalWant strings.Builder
	for _, d := range deltas {
		finalWant.WriteString(d.ReasoningContent)
	}
	if got := reasoning[len(reasoning)-1]; got != finalWant.String() {
		t.Errorf("EventReasoning: final snapshot = %q, want %q", got, finalWant.String())
	}
	for i := 1; i < len(reasoning); i++ {
		if reasoning[i] == reasoning[i-1] {
			t.Errorf("EventReasoning emitted byte-identical snapshot at index %d: %q", i, reasoning[i])
		}
	}
	if len(reasoning) >= stall+2 {
		t.Errorf("dedupe failed: got %d events for %d chunks (want < %d)", len(reasoning), len(deltas), len(deltas))
	}
}

func TestIsRepetitionDominated(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty fails open", "", false},
		{"short input fails open", "hello world", false},
		{"distinct report not repetition", distinctReportProse(40), false},
		{"distinct structured lines not repetition", distinctStructuredLines(30), false},
		{"single-line repeated closing tag is repetition", "intro" + strings.Repeat("</konjll>", 60), true},
		{"repeated identical line dominates", strings.Repeat("a long repeated line of text that is clearly echoing.\n", 12), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRepetitionDominated(tt.in); got != tt.want {
				t.Errorf("isRepetitionDominated() = %v, want %v (len=%d)", got, tt.want, len(tt.in))
			}
		})
	}
}

// distinctReportProse builds varied sentences with no shared 60+ char window.
func distinctReportProse(n int) string {
	words := []string{"disk", "uptime", "directory", "file", "cache", "storage", "usage", "report", "system", "health"}
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "The %s value was measured during phase %c at %d units with expected margins.\n",
			words[i%len(words)], rune('a'+i%26), i*7)
	}
	return b.String()
}

// distinctStructuredLines builds fully distinct lines so no 60+ char window
// is shared between any two lines.
func distinctStructuredLines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%c%c%02d %s %s %s\n",
			rune('a'+i%26), rune('A'+i%26), i,
			strings.Repeat(string(rune('x'+i%26)), 10),
			strings.Repeat(string(rune('y'+i%26)), 10),
			strings.Repeat(string(rune('z'+i%26)), 10))
	}
	return b.String()
}

// feedRepeatedContent emits repeated fragments (the degenerate-loop shape)
// until ctx is done or totalLen is reached, whichever comes first.
func feedRepeatedContent(ctx context.Context, fragment string, totalLen int) <-chan *proxy.ChatResponse {
	ch := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(ch)
		sent := 0
		for sent < totalLen {
			select {
			case <-ctx.Done():
				return
			default:
			}
			chunk := fragment
			if remaining := totalLen - sent; remaining < len(chunk) {
				chunk = chunk[:remaining]
			}
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: chunk}}}}
			sent += len(chunk)
		}
	}()
	return ch
}

func newRepetitionTestAgent() *Agent {
	return &Agent{
		config: AgentConfig{Channel: "test", MaxTokens: 1_000_000},
		deps: AgentRuntimeDeps{
			Logger:   logging.NewNopLogger(),
			Observer: func(ev AgentEvent) {},
		},
	}
}

func TestProcessStream_RepetitionGuard_TerminatesDegenerateLoop(t *testing.T) {
	agent := newRepetitionTestAgent()
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	// ~900 chars of a repeated closing tag with no tool calls: the degenerate
	// loop shape from the workspace-health-test incident.
	err := agent.processStream(context.Background(), feedRepeatedContent(context.Background(), "</konjll>", 900), &fullMsg, false, false)
	if err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}
	if fullMsg.Content != "" {
		t.Errorf("repetition guard should discard garbage content, got %d chars", len(fullMsg.Content))
	}
	if fullMsg.ReasoningContent != "[stuck]" {
		t.Errorf("repetition guard should signal [stuck], got %q", fullMsg.ReasoningContent)
	}
}

func TestProcessStream_RepetitionGuard_DoesNotTruncateDistinctContent(t *testing.T) {
	agent := newRepetitionTestAgent()
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	content := distinctReportProse(60)
	ch := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(ch)
		for i := 0; i < len(content); i += 11 {
			end := i + 11
			if end > len(content) {
				end = len(content)
			}
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: content[i:end]}}}}
		}
	}()
	err := agent.processStream(context.Background(), ch, &fullMsg, false, false)
	if err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}
	if fullMsg.Content != content {
		t.Errorf("distinct content should not be truncated: got %d chars, want %d", len(fullMsg.Content), len(content))
	}
	if fullMsg.ReasoningContent != "" {
		t.Errorf("distinct content should not be marked stuck, got %q", fullMsg.ReasoningContent)
	}
}

func TestProcessStream_RepetitionGuard_PreservesNativeToolCalls(t *testing.T) {
	agent := newRepetitionTestAgent()
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	ch := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(ch)
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
			Content:   strings.Repeat("</konjll>", 50),
			ToolCalls: []proxy.ToolCall{{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path":"x"}`}}},
		}}}}
	}()
	err := agent.processStream(context.Background(), ch, &fullMsg, false, false)
	if err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}
	if len(fullMsg.ToolCalls) != 1 {
		t.Errorf("real tool calls must survive the repetition guard, got %d", len(fullMsg.ToolCalls))
	}
}

func TestProcessStream_DurationCap_TerminatesNoProgressStream(t *testing.T) {
	oldDuration := streamMaxDuration
	streamMaxDuration = 50 * time.Millisecond
	t.Cleanup(func() { streamMaxDuration = oldDuration })
	oldBeat := streamHeartbeatInterval
	streamHeartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { streamHeartbeatInterval = oldBeat })

	agent := newRepetitionTestAgent()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	// Distinct, endless content with no tool calls and no natural completion:
	// only the duration cap can terminate it (the repetition guard must not,
	// because the content is varied).
	fragments := strings.Split(distinctReportProse(80), "\n")
	ch := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(ch)
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: fragments[i%len(fragments)]}}}}
		}
	}()
	err := agent.processStream(ctx, ch, &fullMsg, false, false)
	if err != nil {
		t.Fatalf("processStream returned unexpected error: %v", err)
	}
	// The duration cap terminates the stream but preserves the accumulated
	// content (it may be a genuine slow report), so it is not marked [stuck]
	// and no garbage content was cleared.
	if fullMsg.ReasoningContent == "[stuck]" {
		t.Error("duration cap should preserve content, not signal [stuck]")
	}
	if fullMsg.Content == "" {
		t.Error("duration cap should preserve the accumulated content")
	}
	if len(fullMsg.ToolCalls) != 0 {
		t.Errorf("duration cap should only fire with no tool calls, got %d", len(fullMsg.ToolCalls))
	}
}

// TestBuildChatRequestSkipsGrammarWithNativeTools guards against re-attaching
// the GBNF output constraint to requests that carry native `tools`.
// llama.cpp — the only provider that maps to a GBNF constraint — rejects a
// custom grammar combined with tools in one request with HTTP 400 ("Cannot
// use custom grammar constraints with tools."), so a local managed model in
// native mode would fail every tool-using turn before generation starts
// (observed 2026-09-05: assistant execution failed on the local llama.cpp
// path while the same model via an OpenAI-style registration worked).
func TestBuildChatRequestSkipsGrammarWithNativeTools(t *testing.T) {
	newAgent := func(useNativeTools bool) *Agent {
		return &Agent{
			config: AgentConfig{
				UseNativeTools:  useNativeTools,
				ProviderType:    models.ProviderLocal,
				WorkloadClass:   models.WorkloadLocal,
				ReasoningSpec:   reasoning.ReasoningSpec{Mode: reasoning.ModeThinkTokens, Effort: reasoning.EffortMedium, Budget: 512},
				ReasoningBudget: 512,
			},
			deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()},
		}
	}

	tools := []proxy.Tool{{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name: "list_directory",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []any{"path"},
			},
		},
	}}

	t.Run("native tools never carry a grammar", func(t *testing.T) {
		req := newAgent(true).buildChatRequest(nil, tools)
		if len(req.Tools) != len(tools) {
			t.Fatalf("expected %d native tools on request, got %d", len(tools), len(req.Tools))
		}
		if req.Grammar != nil {
			t.Fatal("native-tools request must not attach a GBNF grammar: llama.cpp rejects grammar+tools with 400")
		}
	})

	t.Run("xml text mode request stays grammar-free", func(t *testing.T) {
		req := newAgent(false).buildChatRequest(nil, nil)
		if req.Grammar != nil {
			t.Fatal("xml text-mode request must not attach a GBNF grammar")
		}
	})
}

// TestApplyRequestConfig_RecoveryTempEscalation verifies the recovery
// temperature escalation: a run that is breaking a rut (guardrail-blocked tool
// streak) raises its per-turn temperature up to maxRecoveryTemp, and runs
// without an explicit temperature never start sending one.
func TestApplyRequestConfig_RecoveryTempEscalation(t *testing.T) {
	newAgent := func(temperature float64, escalation float64) *Agent {
		a := &Agent{
			config: AgentConfig{
				Temperature:     temperature,
				ProviderType:    models.ProviderLocal,
				WorkloadClass:   models.WorkloadLocal,
				ReasoningSpec:   reasoning.ReasoningSpec{Mode: reasoning.ModeThinkTokens, Effort: reasoning.EffortMedium, Budget: 512},
				ReasoningBudget: 512,
			},
			deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()},
		}
		if escalation > 0 {
			a.runS = &runSession{recoveryTempEscalation: escalation}
		}
		return a
	}

	t.Run("escalation raises explicit temperature", func(t *testing.T) {
		req := proxy.ChatRequest{}
		newAgent(0.1, 0.3).applyRequestConfig(&req)
		if req.Temperature != 0.4 {
			t.Errorf("temperature = %v, want 0.4 (0.1 + 0.3)", req.Temperature)
		}
	})

	t.Run("escalation capped at maxRecoveryTemp", func(t *testing.T) {
		req := proxy.ChatRequest{}
		newAgent(0.9, 0.3).applyRequestConfig(&req)
		if req.Temperature != maxRecoveryTemp {
			t.Errorf("temperature = %v, want capped %v", req.Temperature, maxRecoveryTemp)
		}
	})

	t.Run("no escalation without a base temperature", func(t *testing.T) {
		req := proxy.ChatRequest{}
		newAgent(0, 0.3).applyRequestConfig(&req)
		if req.Temperature != 0 {
			t.Errorf("temperature = %v, want 0 (server default kept)", req.Temperature)
		}
	})

	t.Run("no escalation without a recovery event", func(t *testing.T) {
		req := proxy.ChatRequest{}
		newAgent(0.1, 0).applyRequestConfig(&req)
		if req.Temperature != 0.1 {
			t.Errorf("temperature = %v, want 0.1 unchanged", req.Temperature)
		}
	})
}

// TestIsGuardrailDenialResult covers the denial-text classifier used by the
// guardrail-blocked tool loop guard.
func TestIsGuardrailDenialResult(t *testing.T) {
	tests := []struct {
		name   string
		result string
		want   bool
	}{
		{"policy block", `{"error":"Guardrail violation: pattern denied. Action blocked by security policy. Do NOT retry, rephrase, or attempt the same outcome via a different path."}`, true},
		{"user denial", `{"error":"Guardrail violation: user denied. Action denied by the user. Do NOT retry, rephrase, or attempt the same outcome via a different path."}`, true},
		{"case-insensitive", `Action BLOCKED BY SECURITY POLICY.`, true},
		{"normal result", `{"output":"ok"}`, false},
		{"tool error", `{"error":"command failed: exit status 1"}`, false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGuardrailDenialResult(tt.result); got != tt.want {
				t.Errorf("isGuardrailDenialResult() = %v, want %v", got, tt.want)
			}
		})
	}
}
