package assistant

import (
	"context"
	"fmt"
	"testing"

	"llm-proxy/internal/core/proxy"
)

// TestReactStrategy_RunTurnSequence is the Phase 1 refactor regression: the
// extracted ReactStrategy must drive the identical turn sequence the
// pre-refactor runSession.run() drove — tool-call turn, tool execution, then a
// natural completion (prior tool result + substantive text). The pre-refactor
// behavior is pinned by the wider agent_test.go suite; this test pins the
// extracted strategy explicitly.
func TestReactStrategy_RunTurnSequence(t *testing.T) {
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

	var events []AgentEvent
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopReact,
		Observer:     func(ev AgentEvent) { events = append(events, ev) },
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Report\nTask completed successfully." {
		t.Errorf("expected final report reply, got %q", reply)
	}
	if engine.Calls != 1 {
		t.Errorf("expected 1 tool execution, got %d", engine.Calls)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (tool turn + completion), got %d", callCount)
	}

	// Step notifications must fire per turn (notifyStepStart/notifyThinking).
	stepStarts := 0
	completions := 0
	for _, ev := range events {
		if ev.Type == EventStepStart {
			stepStarts++
		}
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == "completed" {
				completions++
			}
		}
	}
	if stepStarts < 2 {
		t.Errorf("expected ≥2 step_start events, got %d", stepStarts)
	}
	if completions != 1 {
		t.Errorf("expected 'completed' lifecycle emitted exactly once, got %d", completions)
	}
}

// TestReactStrategy_ToolErrorContinues pins the cross-strategy convention: the
// react loop records a tool execution failure (engine error) as a tool result
// and continues — the model sees the error in history and still finalizes with
// a report. EvaluatorOptimizerStrategy delegates to ReactStrategy so it
// inherits this; PlanExecuteStrategy's executePlan must match it (see
// plan_execute_guardrail_test.go).
func TestReactStrategy_ToolErrorContinues(t *testing.T) {
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
			msg := proxy.Message{Role: "assistant", Content: "# Report\nTool failed, noted and continued."}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Err: fmt.Errorf("shell execution failed: exit status 2")}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopReact,
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute must not abort on a tool execution error: %v", err)
	}
	if reply != "# Report\nTool failed, noted and continued." {
		t.Errorf("expected final report reply, got %q", reply)
	}
	if engine.Calls != 1 {
		t.Errorf("expected 1 tool execution, got %d", engine.Calls)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (tool turn + completion), got %d", callCount)
	}
}

// TestReactStrategy_DefaultSelection ensures the resolver picks react when no
// strategy is configured (the pre-refactor default), so the whole existing
// suite remains a valid regression harness.
func TestReactStrategy_DefaultSelection(t *testing.T) {
	client := &MockClient{}
	provider := &MockProvider{}
	engine := &MockEngine{}
	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	if got := resolveLoopStrategyName(agent); got != LoopReact {
		t.Errorf("expected default strategy react, got %s", got)
	}
	s := resolveLoopStrategy(agent)
	if s.Name() != LoopReact {
		t.Errorf("expected resolved strategy react, got %s", s.Name())
	}
}
