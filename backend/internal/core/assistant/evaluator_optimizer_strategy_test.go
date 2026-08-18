package assistant

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
)

// TestEvaluatorOptimizerStrategy_GuardNudgeAndCap drives the evaluator-optimizer
// loop: the first natural-completion candidate is interrupted by the evaluator
// nudge (loop continues), the second completion candidate is interrupted again,
// and after the stopGuardAttempts cap the run finalizes — emitting "completed"
// exactly once.
func TestEvaluatorOptimizerStrategy_GuardNudgeAndCap(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			var msg proxy.Message
			switch callCount {
			case 1:
				tc := proxy.ToolCall{ID: "call_1", Type: "function", Function: proxy.FunctionCall{Name: "test_tool", Arguments: `{"key":"val"}`}}
				msg = proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
			case 2:
				msg = proxy.Message{Role: "assistant", Content: "# Report\nVersion one draft"}
			case 3:
				tc := proxy.ToolCall{ID: "call_2", Type: "function", Function: proxy.FunctionCall{Name: "test_tool", Arguments: `{"key":"val2"}`}}
				msg = proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
			case 4:
				msg = proxy.Message{Role: "assistant", Content: "# Report\nVersion two verified"}
			default:
				msg = proxy.Message{Role: "assistant", Content: "# Final report\ntask complete"}
			}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	var events []AgentEvent
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     10,
		LoopStrategy: LoopEvaluatorOptimizer,
		Observer:     func(ev AgentEvent) { events = append(events, ev) },
	})

	reply, history, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Final report\ntask complete" {
		t.Errorf("expected final report reply, got %q", reply)
	}
	if callCount != 5 {
		t.Errorf("expected 5 LLM calls (tool, draft, tool, verified, final), got %d", callCount)
	}

	// Exactly two evaluator nudges were injected (the cap).
	nudges := 0
	for _, m := range history {
		if m.Role == proxy.UserRole && strings.Contains(m.Content, prompts.EvaluatorReviewPrompt) {
			nudges++
		}
	}
	if nudges != 2 {
		t.Errorf("expected exactly 2 evaluator nudges (stopGuardAttempts cap), got %d", nudges)
	}

	// "completed" emitted exactly once.
	completions := 0
	for _, ev := range events {
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == "completed" {
				completions++
			}
		}
	}
	if completions != 1 {
		t.Errorf("expected 'completed' lifecycle emitted exactly once, got %d", completions)
	}
}

// TestEvaluatorOptimizerStrategy_PlainReactNoNudge pins the zero-change
// guarantee: with no stop guards configured, a completion candidate finalizes
// immediately and no evaluator nudge is ever injected.
func TestEvaluatorOptimizerStrategy_PlainReactNoNudge(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				tc := proxy.ToolCall{ID: "call_1", Type: "function", Function: proxy.FunctionCall{Name: "test_tool", Arguments: `{"key":"val"}`}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "# Report\nTask completed successfully."}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, LoopStrategy: LoopReact})
	reply, history, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Report\nTask completed successfully." {
		t.Errorf("expected immediate final report, got %q", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (tool + completion, no nudge), got %d", callCount)
	}
	for _, m := range history {
		if m.Role == proxy.UserRole && strings.Contains(m.Content, prompts.EvaluatorReviewPrompt) {
			t.Error("plain react must not inject an evaluator nudge")
		}
	}
}
