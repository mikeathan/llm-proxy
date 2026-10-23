package assistant

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// The cap on one tool result is sized per call from the room left in the request, never below today's 3000 and never so
// large that one result can take the window. Numbers below follow a real 16k Qwen run: about 40k characters of room in
// all, a first request of about 22.7k.
func TestResultCapFor(t *testing.T) {
	floor := proxy.MaxReturnChars
	cases := []struct {
		name  string
		size  requestSize
		calls int
		want  int
	}{
		{"no budget known keeps today's cap", requestSize{chars: 20000, limit: 0}, 1, floor},
		{"the first single call gets half the room", requestSize{chars: 22700, limit: 40000}, 1, 8650},
		{"five parallel calls share it and fall to the floor", requestSize{chars: 22970, limit: 40000}, 5, floor},
		{"two calls split the room", requestSize{chars: 22700, limit: 40000}, 2, 4325},
		{"a nearly full request gets the floor", requestSize{chars: 39000, limit: 40000}, 1, floor},
		{"an over-full request gets the floor", requestSize{chars: 45000, limit: 40000}, 1, floor},
		{"a big window is capped at the ceiling", requestSize{chars: 10000, limit: 400000}, 1, resultCapCeiling},
		{"zero calls counts as one", requestSize{chars: 22700, limit: 40000}, 0, 8650},
		{"a tiny window never goes below the floor", requestSize{chars: 100, limit: 2000}, 1, floor},
	}
	for _, c := range cases {
		if got := resultCapFor(c.size, c.calls); got != c.want {
			t.Errorf("%s: cap %d, want %d", c.name, got, c.want)
		}
	}
}

// Each result takes half of what is left, so the caps shrink as the request fills and their total can never reach the
// room: a run cannot be pushed past its window by result caps alone (the floor is today's behaviour).
func TestResultCapFor_SuccessiveResultsConvergeAndNeverOverflowTheRoom(t *testing.T) {
	limit, size := 40000, 22700
	total := 0
	for i := 0; i < 20; i++ {
		c := resultCapFor(requestSize{chars: size, limit: limit}, 1)
		if c < proxy.MaxReturnChars {
			t.Fatalf("below the floor: %d", c)
		}
		total += c
		size += c
	}
	floorSteps := 0
	for i, size2 := 0, 22700; i < 20; i++ {
		c := resultCapFor(requestSize{chars: size2, limit: limit}, 1)
		if c == proxy.MaxReturnChars {
			floorSteps++
		}
		size2 += c
	}
	if floorSteps < 10 {
		t.Errorf("the caps must settle at the floor, only %d of 20 steps did", floorSteps)
	}
}

type capEngine struct{ seen []int }

func (e *capEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	n, _ := models.ResultCapFrom(ctx)
	e.seen = append(e.seen, n)
	return strings.Repeat("x", 7000), nil
}

func capAgent(t *testing.T, adaptive bool, size requestSize) (*Agent, *capEngine) {
	t.Helper()
	eng := &capEngine{}
	schema := func(name, arg string) proxy.Tool {
		return proxy.Tool{Type: "function", Function: proxy.FunctionSchema{Name: name, Parameters: map[string]any{
			"type": "object", "properties": map[string]any{arg: map[string]any{"type": "string"}}, "required": []any{arg}}}}
	}
	provider := &MockProvider{Tools: []proxy.Tool{schema("lookup_page", "url"), schema("lookup_doc", "path")}}
	a := NewAgent(&MockClient{}, provider, eng, AgentOptions{MaxSteps: 25, AdaptiveResultCap: adaptive})
	a.runS = newRunSession(a, context.Background(), nil)
	a.noteRequestSize(size)
	return a, eng
}

// One value per turn: every call of a batch gets the same cap, computed before any of them ran, handed to the tool
// through the context and used again when the result is appended, so the tool's own cut and the agent's agree.
func TestToolTurn_OneCapForTheWholeTurnReachesTheToolAndTheResult(t *testing.T) {
	a, eng := capAgent(t, true, requestSize{chars: 22700, limit: 40000})
	var history []proxy.Message
	msg := proxy.Message{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{
		{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "lookup_page", Arguments: `{"url":"https://a.test"}`}},
		{ID: "c2", Type: "function", Function: proxy.FunctionCall{Name: "lookup_page", Arguments: `{"url":"https://b.test"}`}},
	}}
	if _, err := a.processToolCalls(context.Background(), msg, &history, nil); err != nil {
		t.Fatal(err)
	}
	if len(eng.seen) != 2 || eng.seen[0] != 4325 || eng.seen[1] != 4325 {
		t.Errorf("two calls split the room: caps seen by the tool %v, want [4325 4325]", eng.seen)
	}
	for _, m := range history {
		if m.Role == proxy.ToolRole && strings.Contains(m.Content, "SYSTEM TRUNCATED") && len(m.Content) < 4325 {
			t.Errorf("a result is cut shorter than its cap: %d", len(m.Content))
		}
	}
	if a.runS.toolCap != 0 {
		t.Errorf("the cap must not outlive its turn: %d", a.runS.toolCap)
	}
}

func TestToolTurn_ASingleCallOnARoomyRequestKeepsAWholeResult(t *testing.T) {
	a, _ := capAgent(t, true, requestSize{chars: 22700, limit: 40000})
	var history []proxy.Message
	call := proxy.ToolCall{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "lookup_doc", Arguments: `{"path":"a.md"}`}}
	if _, err := a.processToolCalls(context.Background(), proxy.Message{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{call}}, &history, nil); err != nil {
		t.Fatal(err)
	}
	got := history[len(history)-1].Content
	if strings.Contains(got, "TRUNCATED") || len(got) < 7000 {
		t.Errorf("a 7000-character result fits a cap of 8650 and must arrive whole: %d chars, cut=%v", len(got), strings.Contains(got, "TRUNCATED"))
	}
}

func TestToolTurn_WithTheSwitchOffOrNoBudgetItIsToday(t *testing.T) {
	for name, a := range map[string]*Agent{
		"switch off": func() *Agent { a, _ := capAgent(t, false, requestSize{chars: 22700, limit: 40000}); return a }(),
		"no budget":  func() *Agent { a, _ := capAgent(t, true, requestSize{}); return a }(),
	} {
		var history []proxy.Message
		call := proxy.ToolCall{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "lookup_doc", Arguments: `{"path":"a.md"}`}}
		_, _ = a.processToolCalls(context.Background(), proxy.Message{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{call}}, &history, nil)
		got := history[len(history)-1].Content
		if !strings.Contains(got, "SYSTEM TRUNCATED") || len(got) > 3300 {
			t.Errorf("%s: the result must be cut at the default 3000, got %d chars", name, len(got))
		}
	}
}

func TestCurrentResultCap_DefaultOutsideATurn(t *testing.T) {
	a := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5, AdaptiveResultCap: true})
	if got := a.currentResultCap(); got != proxy.MaxReturnChars {
		t.Errorf("no session: %d", got)
	}
	a.runS = newRunSession(a, context.Background(), nil)
	if got := a.currentResultCap(); got != proxy.MaxReturnChars {
		t.Errorf("no turn in progress: %d", got)
	}
}

// The resolved setting (run.TestResolveOptions_AdaptiveResultCap) reaches the agent.
func TestNewAgent_AdaptiveResultCapReachesTheAgent(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		a := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{Logger: logging.NewNopLogger(), AdaptiveResultCap: enabled})
		if a.config.AdaptiveResultCap != enabled {
			t.Errorf("setting %v -> agent %v", enabled, a.config.AdaptiveResultCap)
		}
	}
}
