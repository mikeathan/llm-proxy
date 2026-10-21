package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

func sizedAgent(opts AgentOptions) *Agent {
	return NewAgent(&MockClient{}, &MockProvider{Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{
		Name: "read_file", Description: "read",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]string{"type": "string", "description": strings.Repeat("d", 1000)}}},
	}}}}, &MockEngine{}, opts)
}

func TestRequestLimit(t *testing.T) {
	learned := func(chars, tokens int) *orchestrator.Orchestrator {
		o := &orchestrator.Orchestrator{Ratios: orchestrator.NewTokenRatios()}
		o.Ratios.Observe("m", chars, tokens)
		return o
	}
	cases := []struct {
		name string
		opts AgentOptions
		want int
	}{
		{"local before any observation uses the conservative default", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadLocal, ModelName: "m"}, 30000},
		{"local with a dense learned ratio shrinks the allowance", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadLocal, ModelName: "m", Orchestrator: learned(2800, 1000)}, 28000},
		{"local with prose keeps the nominal allowance", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadLocal, ModelName: "m", Orchestrator: learned(4000, 1000)}, 40000},
		{"cloud is a history allowance and is never scaled", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadCloud, ModelName: "m", Orchestrator: learned(2800, 1000)}, 40000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sizedAgent(tc.opts).requestLimit(); got != tc.want {
				t.Errorf("limit = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMeasureRequest(t *testing.T) {
	history := []proxy.Message{{Role: proxy.UserRole, Content: strings.Repeat("x", 500)}}
	list, _ := sizedAgent(AgentOptions{}).deps.Provider.ListTools(context.Background())
	native, text := true, false
	cases := []struct {
		name        string
		opts        AgentOptions
		wantSchemas bool
	}{
		{"a local native request carries the tool schemas", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadLocal, UseNativeTools: &native}, true},
		{"a text-mode request has them in its messages already", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadLocal, UseNativeTools: &text}, false},
		{"cloud history is measured without them", AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadCloud, UseNativeTools: &native}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := sizedAgent(tc.opts)
			llmTools := list
			if !a.config.UseNativeTools {
				llmTools = nil
			}
			prepared, _ := a.prepareMessagesForTurn(history, list, llmTools)
			want := historyMessagesChars(prepared)
			if tc.wantSchemas {
				want += toolSchemaChars(list)
			}
			size := a.measureRequest(history, list)
			if size.chars != want {
				t.Errorf("chars = %d, want %d", size.chars, want)
			}
			if size.history != 500 {
				t.Errorf("history = %d, want 500", size.history)
			}
		})
	}
}

func TestShrinkHistory(t *testing.T) {
	msg := func(role models.ChatRole, n int) proxy.Message {
		return proxy.Message{Role: role, Content: strings.Repeat("c", n)}
	}
	head := []proxy.Message{msg(proxy.SystemRole, 100), msg(proxy.UserRole, 100), msg(proxy.UserRole, 100)}
	total := func(h []proxy.Message) int {
		n := 0
		for _, m := range h {
			n += messageChars(m)
		}
		return n
	}
	t.Run("a history that fits is returned as it is", func(t *testing.T) {
		h := append(slicesClone(head), msg(proxy.ToolRole, 400))
		got, fits := shrinkHistory(h, 5000)
		if !fits || total(got) != total(h) {
			t.Errorf("fits=%v total=%d", fits, total(got))
		}
	})
	t.Run("the oldest messages are cut first and the newest is kept whole", func(t *testing.T) {
		h := append(slicesClone(head), msg(proxy.ToolRole, 3000), msg(proxy.ToolRole, 3000), msg(proxy.ToolRole, 3000))
		got, fits := shrinkHistory(h, 6500)
		if !fits || total(got) > 6500 {
			t.Fatalf("fits=%v total=%d, want <= 6500", fits, total(got))
		}
		if len(got[len(got)-1].Content) != 3000 {
			t.Errorf("the newest result was cut to %d", len(got[len(got)-1].Content))
		}
		if len(got[3].Content) >= len(got[4].Content) {
			t.Errorf("the oldest (%d) must be cut harder than the next (%d)", len(got[3].Content), len(got[4].Content))
		}
		if total(h) != 300+9000 {
			t.Error("the caller's history must not be modified")
		}
	})
	t.Run("the head is never cut", func(t *testing.T) {
		got, _ := shrinkHistory(append(slicesClone(head), msg(proxy.ToolRole, 3000)), 200)
		for i := range head {
			if got[i].Content != head[i].Content {
				t.Errorf("head message %d was changed", i)
			}
		}
	})
	t.Run("the newest is cut only when everything older is at its floor", func(t *testing.T) {
		h := append(slicesClone(head), msg(proxy.ToolRole, 3000), msg(proxy.ToolRole, 9000))
		got, fits := shrinkHistory(h, 3000)
		if !fits || total(got) > 3000 {
			t.Fatalf("fits=%v total=%d", fits, total(got))
		}
		if len(got[len(got)-1].Content) >= 9000 {
			t.Error("the newest must be cut when nothing else can give")
		}
	})
	t.Run("a history that cannot fit says so", func(t *testing.T) {
		_, fits := shrinkHistory(append(slicesClone(head), msg(proxy.ToolRole, 3000)), 250)
		if fits {
			t.Error("head alone is 300 chars: 250 cannot be met")
		}
	})
	t.Run("tool-call arguments count but are never cut", func(t *testing.T) {
		call := proxy.Message{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{Function: proxy.FunctionCall{Name: "write_file", Arguments: `{"content":"` + strings.Repeat("a", 2000) + `"}`}}}}
		got, fits := shrinkHistory(append(slicesClone(head), call, msg(proxy.ToolRole, 100)), 500)
		if fits {
			t.Error("2 KB of arguments cannot fit 500")
		}
		if got[3].ToolCalls[0].Function.Arguments != call.ToolCalls[0].Function.Arguments {
			t.Error("arguments must stay valid JSON, uncut")
		}
	})
}

func slicesClone(h []proxy.Message) []proxy.Message { return append([]proxy.Message(nil), h...) }

func TestLearnTokenRatio(t *testing.T) {
	usage := &proxy.TokenUsage{PromptTokens: 1000}
	t.Run("a local run teaches the store its model's ratio", func(t *testing.T) {
		o := &orchestrator.Orchestrator{Ratios: orchestrator.NewTokenRatios()}
		a := sizedAgent(AgentOptions{WorkloadClass: models.WorkloadLocal, ModelName: "m", Orchestrator: o})
		a.learnTokenRatio(2800, usage)
		if got := o.Ratios.CharsPerToken("m"); got != 2.8 {
			t.Errorf("ratio = %v, want 2.8", got)
		}
	})
	t.Run("a cloud run does not, because its budget is not a window", func(t *testing.T) {
		o := &orchestrator.Orchestrator{Ratios: orchestrator.NewTokenRatios()}
		sizedAgent(AgentOptions{WorkloadClass: models.WorkloadCloud, ModelName: "m", Orchestrator: o}).learnTokenRatio(2800, usage)
		if got := o.Ratios.CharsPerToken("m"); got != orchestrator.DefaultCharsPerToken {
			t.Errorf("ratio = %v, want the default", got)
		}
	})
	t.Run("no orchestrator and no usage are safe", func(t *testing.T) {
		a := sizedAgent(AgentOptions{WorkloadClass: models.WorkloadLocal, ModelName: "m"})
		a.learnTokenRatio(2800, usage)
		a.learnTokenRatio(2800, nil)
	})
}

func textStream(text string) <-chan *proxy.ChatResponse {
	ch := make(chan *proxy.ChatResponse, 1)
	ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: text}}}}
	close(ch)
	return ch
}

func bulkyHistory(results, size int) []proxy.Message {
	h := []proxy.Message{{Role: proxy.SystemRole, Content: "system prompt"}, {Role: proxy.UserRole, Content: "task: summarise the pages"}}
	for i := range results {
		id := fmt.Sprintf("t%d", i)
		h = append(h,
			proxy.Message{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{{ID: id, Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: "{}"}}}},
			proxy.Message{Role: proxy.ToolRole, ToolCallID: id, Content: strings.Repeat("r", size)})
	}
	return h
}

// A rejected request names how many tokens it was and the window the server enforces, so the recovery cuts the history
// to fit that window in one step instead of keeping a fixed number of messages and trying again.
func TestAgent_ContextOverflowRecoversInOneStep(t *testing.T) {
	const prompt, window, maxTokens = 30000, 16384, 2000
	var sent []int
	client := &MockClient{
		StreamFunc: func(_ context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			sent = append(sent, sentRequestChars(req))
			if len(sent) == 1 {
				return nil, fmt.Errorf("stream: %w", &providers.ContextOverflowError{PromptTokens: prompt, WindowTokens: window})
			}
			return textStream("done"), nil
		},
	}
	o := &orchestrator.Orchestrator{Ratios: orchestrator.NewTokenRatios()}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{}, AgentOptions{
		MaxSteps: 5, ModelName: "m", WorkloadClass: models.WorkloadLocal, ContextBudget: 1_000_000, MaxResponseTokens: maxTokens, Orchestrator: o,
	})
	reply, _, err := agent.Execute(context.Background(), bulkyHistory(10, 6000))
	if err != nil || !strings.Contains(reply, "done") {
		t.Fatalf("reply %q, err %v", reply, err)
	}
	if len(sent) != 2 {
		t.Fatalf("one rejection and one retry expected, got %d requests", len(sent))
	}
	if want := int(float64(sent[0]) * float64(window-maxTokens) / prompt); sent[1] > want {
		t.Errorf("the retry is %d characters, want at most %d (the window less the output reserve)", sent[1], want)
	}
	if got, want := o.Ratios.CharsPerToken("m"), float64(sent[0])/prompt; got != want {
		t.Errorf("learned ratio = %v, want %v from the rejected request", got, want)
	}
}

// When even the part that cannot be cut (system prompt, task, tool schemas) is over the window, no recovery can help,
// so the run ends at once and says why instead of retrying.
func TestAgent_ContextOverflowFailsFastWhenTheFixedPartIsTooBig(t *testing.T) {
	calls := 0
	client := &MockClient{
		StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			calls++
			return nil, &providers.ContextOverflowError{PromptTokens: 30000, WindowTokens: 16384}
		},
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{}, AgentOptions{
		MaxSteps: 5, ModelName: "m", WorkloadClass: models.WorkloadLocal, ContextBudget: 1_000_000, MaxResponseTokens: 2000,
	})
	history := []proxy.Message{{Role: proxy.SystemRole, Content: "s"}, {Role: proxy.UserRole, Content: strings.Repeat("t", 200_000)}}
	_, _, err := agent.Execute(context.Background(), history)
	if !errors.Is(err, models.ErrPromptTooLarge) {
		t.Fatalf("want ErrPromptTooLarge, got %v", err)
	}
	if calls != 1 {
		t.Errorf("a request that cannot fit must not be retried, got %d calls", calls)
	}
}

// BenchmarkMeasureRequest is the per-turn cost of sizing the request: preparing the messages and marshalling the tool
// schemas, for a 40-message history and twelve tools.
func BenchmarkMeasureRequest(b *testing.B) {
	tools := make([]proxy.Tool, 12)
	for i := range tools {
		tools[i] = proxy.Tool{Type: "function", Function: proxy.FunctionSchema{
			Name: fmt.Sprintf("tool_%d", i), Description: strings.Repeat("d", 200),
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]string{"type": "string", "description": strings.Repeat("p", 300)}}},
		}}
	}
	native := true
	a := NewAgent(&MockClient{}, &MockProvider{Tools: tools}, &MockEngine{}, AgentOptions{ContextBudget: 40000, WorkloadClass: models.WorkloadLocal, UseNativeTools: &native})
	history := bulkyHistory(40, 3000)
	b.ReportAllocs()
	for b.Loop() {
		a.measureRequest(history, tools)
	}
}

// The sieve prunes to the same limit the over-budget decision used, so a local model whose tokens cost more than the
// nominal ratio is pruned further, not just told it is over.
func TestApplyPhysicalSieve_UsesTheScaledLimit(t *testing.T) {
	o := &orchestrator.Orchestrator{Ratios: orchestrator.NewTokenRatios()}
	o.Ratios.Observe("m", 2000, 1000) // 2.0 chars per token: half the nominal 4.0
	a := sizedAgent(AgentOptions{ContextBudget: 10000, WorkloadClass: models.WorkloadLocal, ModelName: "m", Orchestrator: o})
	got := a.applyPhysicalSieve(bulkyHistory(20, 300), 0)
	if total := historyMessagesChars(got); total > 5000 {
		t.Errorf("history is %d characters, want at most the scaled limit of 5000", total)
	}
}

// An overflow that reports the server's window shows the run started with a stale budget (a model whose metadata fell
// back to the training window); the run lowers its budget to that window, so the turns after the recovery are sized to
// it and the next large result does not overflow it again. It never raises a budget.
func TestAgent_ContextOverflowAdoptsTheReportedWindow(t *testing.T) {
	run := func(t *testing.T, budget, maxTokens int) *Agent {
		t.Helper()
		calls := 0
		client := &MockClient{StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			calls++
			if calls == 1 {
				return nil, fmt.Errorf("stream: %w", &providers.ContextOverflowError{PromptTokens: 17155, WindowTokens: 16384})
			}
			return textStream("done"), nil
		}}
		o := &orchestrator.Orchestrator{Ratios: orchestrator.NewTokenRatios()}
		agent := NewAgent(client, &MockProvider{}, &MockEngine{}, AgentOptions{
			MaxSteps: 5, ModelName: "m", WorkloadClass: models.WorkloadLocal, ContextBudget: budget, MaxResponseTokens: maxTokens, Orchestrator: o,
		})
		if _, _, err := agent.Execute(context.Background(), bulkyHistory(10, 6000)); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return agent
	}

	t.Run("a budget from the training window is lowered to the server's", func(t *testing.T) {
		agent := run(t, 699052, 87381) // what a 262144-token training window gives, against a 16384-token server
		if agent.config.ContextBudget != 43692 || agent.config.MaxTokens != 5461 {
			t.Errorf("budget %d, max tokens %d; want 43692 and 5461 (a 16384-token window)", agent.config.ContextBudget, agent.config.MaxTokens)
		}
	})
	t.Run("a budget already inside the window is left alone", func(t *testing.T) {
		agent := run(t, 30000, 2000)
		if agent.config.ContextBudget != 30000 || agent.config.MaxTokens != 2000 {
			t.Errorf("budget %d, max tokens %d; want them unchanged", agent.config.ContextBudget, agent.config.MaxTokens)
		}
	})
}
