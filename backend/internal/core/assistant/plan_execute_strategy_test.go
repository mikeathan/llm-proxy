package assistant

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/proxy"
)

// TestPlanExecuteStrategy_Success drives plan generation → sequential execution
// → the shared finalizeReport finalization turn, and asserts the shared
// completion path fires the "completed" lifecycle exactly once.
func TestPlanExecuteStrategy_Success(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
					Role:    "assistant",
					Content: `{"description": "test plan", "steps": [{"tool": "test_tool", "description": "step 1", "args": {"key": "val"}}]}`,
				}}}}, nil
			}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    "assistant",
				Content: "# Report\nPlan executed, all steps succeeded.",
			}}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	var events []AgentEvent
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopPlanExecute,
		Observer:     func(ev AgentEvent) { events = append(events, ev) },
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Report\nPlan executed, all steps succeeded." {
		t.Errorf("expected synthesized report (not '[Plan execution complete]'), got %q", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (plan generation + finalization turn), got %d", callCount)
	}
	if engine.Calls != 1 {
		t.Errorf("expected 1 plan step executed, got %d", engine.Calls)
	}

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

// TestPlanExecuteStrategy_EmitsPlanningEvent drives plan generation through the
// strategy and asserts the shared primitive emits a visible "planning" message
// and a neutral agent_thinking lifecycle signal before the plan is generated, so
// the assistant panel is never blank during the synchronous pre-loop LLM call.
func TestPlanExecuteStrategy_EmitsPlanningEvent(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    "assistant",
				Content: `{"description": "test plan", "steps": [{"tool": "test_tool", "description": "step 1", "args": {"key": "val"}}]}`,
			}}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	var events []AgentEvent
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopPlanExecute,
		Observer:     func(ev AgentEvent) { events = append(events, ev) },
	})

	if _, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	planningMsg := false
	thinkingLifecycle := false
	for _, ev := range events {
		if ev.Type == EventMessage {
			if msg, ok := ev.Payload.(proxy.Message); ok && msg.Content == "🧠 Generating execution plan…" {
				planningMsg = true
			}
		}
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == PhaseAgentThinking {
				thinkingLifecycle = true
			}
		}
	}
	if !planningMsg {
		t.Error("expected a 'Generating execution plan…' EventMessage to be emitted")
	}
	if !thinkingLifecycle {
		t.Error("expected an agent_thinking lifecycle signal to be emitted before plan generation")
	}
}

// TestRunSession_GeneratePlan_BoundedByTimeout proves the shared plan-generation
// primitive bounds the synchronous pre-loop LLM call by context: a hung upstream
// that never returns must surface as a prompt context error instead of blocking
// the run indefinitely. The mock StreamFunc (the primary plan-gen path) blocks
// on ctx.Done() to simulate a stalled provider; the short outer deadline forces
// the timeout to fire, and the Chat fallback stays bounded too. Fully
// synchronous — no goroutines, no shared mutable state.
func TestRunSession_GeneratePlan_BoundedByTimeout(t *testing.T) {
	entered := make(chan struct{}, 2)
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	run := newRunSession(agent, context.Background(), nil)

	outerCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, planErr := run.generatePlan(outerCtx, provider.Tools, "do the task")
	if time.Since(start) > 5*time.Second {
		t.Fatal("generatePlan took too long — the LLM call is not bounded by the context")
	}
	select {
	case <-entered:
	default:
		t.Fatal("expected the plan-generation LLM call to be entered before the timeout fired")
	}
	if planErr == nil {
		t.Fatal("expected generatePlan to return an error on context expiry")
	}
}

// TestPlanExecuteStrategy_SurfacesPlanningReasoning drives plan generation
// through the shared streaming primitive and asserts the planner's reasoning is
// surfaced as EventReasoning while the plan still executes (engine called) — the
// pre-loop call is no longer opaque.
func TestPlanExecuteStrategy_SurfacesPlanningReasoning(t *testing.T) {
	streamCall := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCall++
			ch := make(chan *proxy.ChatResponse, 2)
			if streamCall == 1 {
				// Plan-generation stream: a reasoning delta, then the plan JSON.
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					ReasoningContent: "I should list the steps before executing.",
				}}}}
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					Content: `{"description": "test plan", "steps": [{"tool": "test_tool", "description": "step 1", "args": {"key": "val"}}]}`,
				}}}}
			} else {
				// Finalization turn streams the report text.
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					Content: "# Report\nPlan executed via streamed reasoning.",
				}}}}
			}
			close(ch)
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	var events []AgentEvent
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopPlanExecute,
		Observer:     func(ev AgentEvent) { events = append(events, ev) },
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Report\nPlan executed via streamed reasoning." {
		t.Errorf("expected report, got %q", reply)
	}
	if engine.Calls != 1 {
		t.Errorf("expected 1 plan step executed, got %d", engine.Calls)
	}

	reasoningSeen := false
	for _, ev := range events {
		if ev.Type == EventReasoning {
			if text, ok := ev.Payload.(string); ok && text == "I should list the steps before executing." {
				reasoningSeen = true
			}
		}
	}
	if !reasoningSeen {
		t.Error("expected the planner's reasoning to be surfaced as EventReasoning")
	}
}

// TestPlanExecuteStrategy_GenerationFailureFallsBack drives a plan-generation
// LLM error through the strategy and asserts the react loop takes over.
func TestPlanExecuteStrategy_GenerationFailureFallsBack(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("upstream outage")
			}
			if callCount == 2 {
				tc := proxy.ToolCall{ID: "call_1", Type: "function", Function: proxy.FunctionCall{Name: "test_tool", Arguments: `{"key":"val"}`}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "# Report\nRecovered via react loop."}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, LoopStrategy: LoopPlanExecute})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Report\nRecovered via react loop." {
		t.Errorf("expected react-loop fallback reply, got %q", reply)
	}
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls (failed plan + react tool turn + completion), got %d", callCount)
	}
}

// TestPlanExecuteStrategy_WrongParamNameFailsFast drives a plan whose
// write_file step guesses a parameter name (file_path) instead of the schema's
// required `path`. The plan step must fail fast at validation — the tool never
// executes — mirroring the react loop's required-param enforcement. Without
// this, the empty path would resolve to the workspace root and the run fails
// with a confusing "is a directory" error.
func TestPlanExecuteStrategy_WrongParamNameFailsFast(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    "assistant",
				Content: `{"description": "test plan", "steps": [{"tool": "write_file", "description": "step 1", "args": {"file_path": "a.txt", "content": "hi"}}]}`,
			}}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name:        "write_file",
				Description: "save a file",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
					},
					"required": []any{"path", "content"},
				},
			},
		}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopPlanExecute,
	})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err == nil {
		t.Fatal("expected plan execution to fail on a missing required parameter")
	}
	if !strings.Contains(err.Error(), "missing required parameter 'path'") {
		t.Errorf("expected missing-required error, got %v", err)
	}
	if engine.Calls != 0 {
		t.Errorf("expected no tool execution for an invalid plan step, got %d", engine.Calls)
	}
}

// TestPlanExecuteStrategy_NoToolsFallsBack drives the empty-tool-set fallback.
func TestPlanExecuteStrategy_NoToolsFallsBack(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    "assistant",
				Content: "No tools available.",
			}}}}, nil
		},
	}
	provider := &MockProvider{Tools: []proxy.Tool{}}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, LoopStrategy: LoopPlanExecute})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "No tools available." {
		t.Errorf("expected react-loop reply with no tools, got %q", reply)
	}
}

// TestPlanExecuteStrategy_NoUserMessageFallsBack drives the no-user-message
// fallback: with no user-role message to plan for, the react loop runs.
func TestPlanExecuteStrategy_NoUserMessageFallsBack(t *testing.T) {
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
			msg := proxy.Message{Role: "assistant", Content: "# Report\nRan react loop."}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5, LoopStrategy: LoopPlanExecute})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Report\nRan react loop." {
		t.Errorf("expected react-loop reply, got %q", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 react-loop LLM calls (no plan attempted), got %d", callCount)
	}
}

// TestPlanExecuteStrategy_StepFailureStillReports drives the full strategy with
// a plan whose single step fails at execution time (engine error). The step
// failure must be recorded and the strategy must still produce the final
// report via finalizeReport — a failed step is a reported outcome, never a
// run-killer. Cross-strategy convention: react (and evaluator-optimizer, which
// delegates to it) already record-and-continue; see
// TestReactStrategy_ToolErrorContinues and TestExecutePlan_StepExecutionErrorContinues.
func TestPlanExecuteStrategy_StepFailureStillReports(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
					Role:    "assistant",
					Content: `{"description": "test plan", "steps": [{"tool": "test_tool", "description": "step 1", "args": {"key": "val"}}]}`,
				}}}}, nil
			}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{
				Role:    "assistant",
				Content: "# Report\nStep failed, noted; task summarized.",
			}}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}}},
	}
	engine := &MockEngine{Err: fmt.Errorf("shell execution failed: exit status 2")}

	var events []AgentEvent
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:     5,
		LoopStrategy: LoopPlanExecute,
		Observer:     func(ev AgentEvent) { events = append(events, ev) },
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute must not abort when a plan step fails: %v", err)
	}
	if reply != "# Report\nStep failed, noted; task summarized." {
		t.Errorf("expected final report reply, got %q", reply)
	}
	if engine.Calls != 1 {
		t.Errorf("expected 1 plan step executed, got %d", engine.Calls)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (plan generation + finalization), got %d", callCount)
	}
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
