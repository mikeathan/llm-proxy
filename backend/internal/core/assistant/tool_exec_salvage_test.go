package assistant

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

func TestExtractTruncatedJSONField(t *testing.T) {
	longBody := strings.Repeat("x", salvageMinContentLen) + " report tail"
	tests := []struct {
		name  string
		raw   string
		field string
		want  string
		// wantContains checks substring when full equality is awkward
		wantContains string
		wantPrefix   string
	}{
		{
			name:       "network-scan truncated no-space colon",
			raw:        `{"content":"# Network Reconnaissance Report\n**Task ID:** network-recon\n\n| Host | Port |\n|------|------|\n| 192.168.50.10 | 22 |\n\n### Hardening\n- Enable NLA\n- Implement IP-based`,
			field:      "content",
			wantPrefix: "# Network Reconnaissance Report",
			wantContains: "Implement IP-based",
		},
		{
			name:  "spaced colon complete",
			raw:   `{"content": "hello spaced"}`,
			field: "content",
			want:  "hello spaced",
		},
		{
			name:  "space around colon",
			raw:   `{"content" : "hello pad"}`,
			field: "content",
			want:  "hello pad",
		},
		{
			name:  "escaped quotes and newlines",
			raw:   `{"content":"Service says \"No banner\"\nand done"}`,
			field: "content",
			want:  "Service says \"No banner\"\nand done",
		},
		{
			name:  "complete object stops at closing quote",
			raw:   `{"path":"report.md","content":"full body here","extra":1}`,
			field: "content",
			want:  "full body here",
		},
		{
			name:  "path field from complete json",
			raw:   `{"path":"report.md","content":"x"}`,
			field: "path",
			want:  "report.md",
		},
		{
			name:  "missing field",
			raw:   `{"path":"report.md"}`,
			field: "content",
			want:  "",
		},
		{
			name:  "empty raw",
			raw:   "",
			field: "content",
			want:  "",
		},
		{
			name:  "field name only as other key substring",
			raw:   `{"mycontent":"nope","path":"p"}`,
			field: "content",
			want:  "",
		},
		{
			name:         "markdown table unicode",
			raw:          `{"content":"# R\n| Hōst | Port |\n|------|------|\n| café | 22 |\n- Implement IP-based`,
			field:        "content",
			wantPrefix:   "# R",
			wantContains: "café",
		},
		{
			name:  "trailing backslash truncated escape",
			raw:   `{"content":"hello world\`,
			field: "content",
			want:  "hello world",
		},
		{
			name:         "long truncated body",
			raw:          `{"content":"` + longBody,
			field:        "content",
			wantPrefix:   "xxx",
			wantContains: "report tail",
		},
		{
			name:  "tabs and escaped slash-ish",
			raw:   `{"content":"a\tb\\nc"}`,
			field: "content",
			want:  "a\tb\\nc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTruncatedJSONField(tt.raw, tt.field)
			if tt.want != "" && got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("prefix: got %q want prefix %q", clip(got, 60), tt.wantPrefix)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Fatalf("contains %q missing in %q", tt.wantContains, clip(got, 80))
			}
			if tt.want == "" && tt.wantPrefix == "" && tt.wantContains == "" && got != "" {
				t.Fatalf("want empty, got %q", got)
			}
		})
	}
}

func TestExtractToolArgField(t *testing.T) {
	t.Run("valid json prefers unmarshal", func(t *testing.T) {
		raw := `{"path":"report.md","content":"hello world report body that is long enough"}`
		if got := extractToolArgField(raw, "path"); got != "report.md" {
			t.Fatalf("path: got %q", got)
		}
		if got := extractToolArgField(raw, "content"); !strings.HasPrefix(got, "hello") {
			t.Fatalf("content: got %q", got)
		}
	})
	t.Run("invalid falls back to truncated extract", func(t *testing.T) {
		raw := `{"content":"# Report\nline2 and more text that continues`
		got := extractToolArgField(raw, "content")
		if !strings.HasPrefix(got, "# Report") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("non-string field type ignored", func(t *testing.T) {
		raw := `{"content":123}`
		if got := extractToolArgField(raw, "content"); got != "" {
			t.Fatalf("want empty for non-string, got %q", got)
		}
	})
}

func TestSalvageTruncatedWrite_PersistsWhenPathPresent(t *testing.T) {
	long := strings.Repeat("y", salvageMinContentLen) + "\n# Report body"
	engine := &MockEngine{Result: "File written successfully"}
	agent := &Agent{
		deps: AgentRuntimeDeps{
			Engine: engine,
			Logger: logging.NewNopLogger(),
			// Guardrails nil → skip path policy; tests Engine persist wiring only.
		},
	}
	tc := proxy.ToolCall{
		ID:   "call_w",
		Type: "function",
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"path":"out/report.md","content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	report, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled {
		t.Fatal("expected salvage handled")
	}
	if !strings.Contains(report, "Report body") {
		t.Fatalf("unexpected report: %q", clip(report, 60))
	}
	if engine.Calls != 1 {
		t.Fatalf("expected 1 engine call, got %d", engine.Calls)
	}
	if engine.LastCall.Function.Name != models.ToolFileWrite {
		t.Fatalf("tool=%q", engine.LastCall.Function.Name)
	}
	if !strings.Contains(engine.LastCall.Function.Arguments, "out/report.md") {
		t.Fatalf("path missing in args: %q", engine.LastCall.Function.Arguments)
	}
	if !strings.Contains(engine.LastCall.Function.Arguments, "Report body") {
		t.Fatalf("content missing in args: %q", clip(engine.LastCall.Function.Arguments, 80))
	}
}

func TestSalvageTruncatedWrite_NoPathSkipsEngine(t *testing.T) {
	long := strings.Repeat("z", salvageMinContentLen) + " content only"
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		deps: AgentRuntimeDeps{Engine: engine, Logger: logging.NewNopLogger()},
	}
	tc := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	report, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled || !strings.Contains(report, "content only") {
		t.Fatalf("handled=%v report=%q", handled, clip(report, 40))
	}
	if engine.Calls != 0 {
		t.Fatalf("engine must not run without path, calls=%d", engine.Calls)
	}
}

func TestSalvageTruncatedWrite_PersistFailStillHandled(t *testing.T) {
	long := strings.Repeat("q", salvageMinContentLen) + " still complete"
	engine := &MockEngine{Err: fmt.Errorf("disk full")}
	agent := &Agent{
		deps: AgentRuntimeDeps{Engine: engine, Logger: logging.NewNopLogger()},
	}
	tc := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"path":"r.md","content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	report, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled {
		t.Fatal("persist failure must still handle salvage")
	}
	if !strings.Contains(report, "still complete") {
		t.Fatalf("report=%q", clip(report, 40))
	}
	if engine.Calls != 1 {
		t.Fatalf("expected engine attempt, got %d", engine.Calls)
	}
}

func TestSalvageTruncatedWrite_AppendUsesAppendTool(t *testing.T) {
	long := strings.Repeat("a", salvageMinContentLen) + " append tail"
	engine := &MockEngine{Result: "ok"}
	agent := &Agent{
		deps: AgentRuntimeDeps{Engine: engine, Logger: logging.NewNopLogger()},
	}
	tc := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileAppend,
			Arguments: `{"path":"log.md","content":"` + long,
		},
	}
	var history []proxy.Message
	var mu sync.Mutex
	_, handled := agent.salvageTruncatedWrite(context.Background(), tc, &history, &mu)
	if !handled {
		t.Fatal("expected handled")
	}
	if engine.LastCall.Function.Name != models.ToolFileAppend {
		t.Fatalf("expected append_file, got %q", engine.LastCall.Function.Name)
	}
}

func TestTrySalvageWriteContent(t *testing.T) {
	long := strings.Repeat("y", salvageMinContentLen)
	longPlus := long + " tail"

	tests := []struct {
		name    string
		tool    string
		args    string
		wantLen int // 0 = no salvage; >0 = min length of salvaged
		wantHas string
	}{
		{
			name: "write short no salvage",
			tool: models.ToolFileWrite,
			args: `{"content":"too short"}`,
		},
		{
			name:    "write exact min length",
			tool:    models.ToolFileWrite,
			args:    `{"content":"` + long,
			wantLen: salvageMinContentLen,
		},
		{
			name: "write one under min",
			tool: models.ToolFileWrite,
			args: `{"content":"` + strings.Repeat("z", salvageMinContentLen-1),
		},
		{
			name:    "write long truncated",
			tool:    models.ToolFileWrite,
			args:    `{"content":"` + longPlus,
			wantLen: salvageMinContentLen,
			wantHas: "tail",
		},
		{
			name:    "append long truncated",
			tool:    models.ToolFileAppend,
			args:    `{"content":"` + longPlus,
			wantLen: salvageMinContentLen,
			wantHas: "tail",
		},
		{
			name: "scan tool never salvages",
			tool: models.ToolNetworkScan,
			args: `{"content":"` + longPlus,
		},
		{
			name: "read tool never salvages",
			tool: models.ToolFileRead,
			args: `{"content":"` + longPlus,
		},
		{
			name:    "valid complete write still extracts if long enough",
			tool:    models.ToolFileWrite,
			args:    `{"path":"r.md","content":"` + longPlus + `"}`,
			wantLen: salvageMinContentLen,
			wantHas: "tail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := proxy.ToolCall{Function: proxy.FunctionCall{Name: tt.tool, Arguments: tt.args}}
			got := trySalvageWriteContent(tc)
			if tt.wantLen == 0 {
				if got != "" {
					t.Fatalf("want no salvage, got len=%d", len(got))
				}
				return
			}
			if len(strings.TrimSpace(got)) < tt.wantLen {
				t.Fatalf("got len=%d want >= %d", len(got), tt.wantLen)
			}
			if tt.wantHas != "" && !strings.Contains(got, tt.wantHas) {
				t.Fatalf("missing %q in salvaged", tt.wantHas)
			}
		})
	}
}

func TestHandleToolCallParseError_CapsSyntaxStreak(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, nil)
	err := fmt.Errorf(`llm completion failed: LLM chat error 500: {"error":{"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error: missing closing quote"}}`)

	for i := 1; i < sessionMaxSyntaxParseRetries; i++ {
		if s.handleToolCallParseError(err) {
			t.Fatalf("should not give up on attempt %d", i)
		}
	}
	if !s.handleToolCallParseError(err) {
		t.Fatal("should give up after sessionMaxSyntaxParseRetries")
	}
	if s.syntaxParseStreak != sessionMaxSyntaxParseRetries {
		t.Fatalf("streak=%d want %d", s.syntaxParseStreak, sessionMaxSyntaxParseRetries)
	}
}

func TestHandleToolCallParseError_NonSyntaxDoesNotCapSameWay(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, nil)
	// Length-style server error (not missing closing quote / unexpected end alone in isJSONSyntaxError path)
	// isJSONSyntaxError also matches "unexpected end" — use a generic tool-call parse error without those.
	err := fmt.Errorf(`llm completion failed: LLM chat error 500: Failed to parse tool call arguments as JSON: invalid character`)

	// Non-syntax path should never return giveUp=true from syntax cap
	for i := 0; i < sessionMaxSyntaxParseRetries+2; i++ {
		if s.handleToolCallParseError(err) {
			t.Fatalf("non-syntax path must not give up via syntax cap (i=%d)", i)
		}
	}
	if s.syntaxParseStreak != 0 {
		t.Fatalf("syntax streak should stay 0, got %d", s.syntaxParseStreak)
	}
	if s.totalErrorStreak == 0 {
		t.Fatal("totalErrorStreak should increase on non-syntax path")
	}
}

func TestResetParseErrorState_ClearsSyntaxStreak(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, nil)
	err := fmt.Errorf(`Failed to parse tool call arguments as JSON: missing closing quote`)
	_ = s.handleToolCallParseError(err)
	if s.syntaxParseStreak == 0 {
		t.Fatal("expected streak > 0")
	}
	s.resetParseErrorState()
	if s.syntaxParseStreak != 0 {
		t.Fatalf("reset should clear syntaxParseStreak, got %d", s.syntaxParseStreak)
	}
}

func TestBestAvailableAnswer(t *testing.T) {
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.UserRole, Content: "go"},
		{Role: proxy.AssistantRole, Content: "short"},
		{Role: proxy.ToolRole, Content: "data"},
		{Role: proxy.AssistantRole, Content: "This is a sufficiently long final answer for recovery."},
	})
	got := s.bestAvailableAnswer()
	if !strings.Contains(got, "sufficiently long") {
		t.Fatalf("got %q", got)
	}

	s2 := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, Content: "tiny"},
	})
	if s2.bestAvailableAnswer() != "" {
		t.Fatal("short assistant content must not count")
	}
}

func TestSalvageReportFromToolCallsAndHistory(t *testing.T) {
	long := strings.Repeat("r", salvageMinContentLen) + " report body"
	truncWrite := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      models.ToolFileWrite,
			Arguments: `{"content":"` + long,
		},
	}
	if got := salvageReportFromToolCalls([]proxy.ToolCall{truncWrite}); !strings.Contains(got, "report body") {
		t.Fatalf("tool calls salvage: got %q", clip(got, 40))
	}
	if got := salvageReportFromToolCalls([]proxy.ToolCall{{
		Function: proxy.FunctionCall{Name: models.ToolNetworkScan, Arguments: `{"content":"` + long},
	}}); got != "" {
		t.Fatalf("scan must not salvage, got %q", got)
	}

	hist := []proxy.Message{
		{Role: proxy.UserRole, Content: "scan"},
		{Role: proxy.AssistantRole, Content: "planning only text that is long enough"},
		{
			Role: proxy.AssistantRole,
			Content: "writing report",
			ToolCalls: []proxy.ToolCall{truncWrite},
		},
	}
	if got := salvageReportFromHistory(hist); !strings.Contains(got, "report body") {
		t.Fatalf("history salvage: got %q", clip(got, 40))
	}
	if got := salvageReportFromHistory([]proxy.Message{
		{Role: proxy.AssistantRole, Content: "only text long enough for fallback"},
	}); got != "" {
		t.Fatalf("no tool calls: want empty, got %q", got)
	}
}

func TestResolveFallbackAnswer_Priority(t *testing.T) {
	long := strings.Repeat("z", salvageMinContentLen) + " salvaged report"
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}

	// Salvage wins over assistant prose.
	s := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, Content: "This is a sufficiently long final answer for recovery."},
		{
			Role: proxy.AssistantRole,
			Content: "I will write the file now.",
			ToolCalls: []proxy.ToolCall{{
				Function: proxy.FunctionCall{
					Name:      models.ToolFileWrite,
					Arguments: `{"content":"` + long,
				},
			}},
		},
	})
	got := s.resolveFallbackAnswer()
	if !strings.Contains(got, "salvaged report") {
		t.Fatalf("salvage should win priority, got %q", clip(got, 60))
	}

	// Text-only fallback.
	s2 := newRunSession(agent, nil, []proxy.Message{
		{Role: proxy.AssistantRole, Content: "This is a sufficiently long final answer for recovery."},
	})
	if got := s2.resolveFallbackAnswer(); !strings.Contains(got, "sufficiently long") {
		t.Fatalf("text fallback: got %q", got)
	}

	// Empty history.
	s3 := newRunSession(agent, nil, nil)
	if got := s3.resolveFallbackAnswer(); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

func TestHandleTurnError_GiveUpUsesFallbackSalvage(t *testing.T) {
	long := strings.Repeat("q", salvageMinContentLen) + " history report"
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
	s := newRunSession(agent, nil, []proxy.Message{
		{
			Role: proxy.AssistantRole,
			ToolCalls: []proxy.ToolCall{{
				Function: proxy.FunctionCall{
					Name:      models.ToolFileWrite,
					Arguments: `{"content":"` + long,
				},
			}},
		},
	})
	s.syntaxParseStreak = sessionMaxSyntaxParseRetries - 1
	s.starvationCount = 0
	err := fmt.Errorf(`llm completion failed: Failed to parse tool call arguments as JSON: missing closing quote`)

	done, reply, outErr := s.handleTurnError(err)
	if !done || outErr != nil {
		t.Fatalf("done=%v err=%v", done, outErr)
	}
	if !strings.Contains(reply, "history report") {
		t.Fatalf("expected salvaged history report, got %q", clip(reply, 60))
	}
}

func TestScanJSONStringBody(t *testing.T) {
	if got := scanJSONStringBody(`hello\"world" rest`); got != `hello\"world` {
		t.Fatalf("got %q", got)
	}
	if got := scanJSONStringBody(`no close`); got != `no close` {
		t.Fatalf("truncated got %q", got)
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
