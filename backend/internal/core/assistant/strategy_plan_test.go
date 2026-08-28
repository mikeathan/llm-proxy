package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

func TestExecutionPlan_JSONRoundTrip(t *testing.T) {
	plan := ExecutionPlan{
		Description: "Test plan",
		Steps: []ExecutionStep{
			{
				ToolName:    "read_file",
				Description: "Read the input file",
				Parameters:  map[string]interface{}{"path": "test.txt"},
			},
			{
				ToolName:    "write_file",
				Description: "Write output",
				Parameters:  map[string]interface{}{"path": "out.txt", "content": "done"},
			},
		},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var recovered ExecutionPlan
	if err := json.Unmarshal(data, &recovered); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if recovered.Description != "Test plan" {
		t.Errorf("expected 'Test plan', got '%s'", recovered.Description)
	}
	if len(recovered.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(recovered.Steps))
	}
	if recovered.Steps[0].ToolName != "read_file" {
		t.Errorf("expected step 0 tool 'read_file', got '%s'", recovered.Steps[0].ToolName)
	}
}

func TestNewExecutionPlanStrategy(t *testing.T) {
	tools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
	}
	client := &MockClient{}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, tools, logger)
	if strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
	if strategy.llm != client {
		t.Error("expected llm client to match")
	}
	if len(strategy.tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(strategy.tools))
	}
}

func TestExecutionPlanStrategy_Generate_FailedChat(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{}, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{}, logger)
	_, err := strategy.Generate(context.Background(), "test task")
	if err == nil {
		t.Error("expected error when LLM returns no choices")
	}
}

func TestExecutionPlanStrategy_Generate_InvalidJSON(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Content: "not valid json at all"}},
				},
			}, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{}, logger)
	_, err := strategy.Generate(context.Background(), "test task")
	if err == nil {
		t.Error("expected error when LLM returns invalid JSON")
	}
}

func TestExecutionPlanStrategy_Generate_Success(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{
						Message: proxy.Message{
							Content: `{"description": "Test", "steps": [{"tool": "read_file", "description": "Read", "args": {"path": "test.txt"}}]}`,
						},
					},
				},
			}, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
	}, logger)
	plan, err := strategy.Generate(context.Background(), "test task")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if plan.Description != "Test" {
		t.Errorf("expected 'Test', got '%s'", plan.Description)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "read_file" {
		t.Errorf("expected 'read_file', got '%s'", plan.Steps[0].ToolName)
	}
}

func TestExecutionPlanStrategy_Generate_MarkdownCodeBlock(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{
						Message: proxy.Message{
							Content: "```json\n{\"description\": \"Test\", \"steps\": [{\"tool\": \"search\", \"description\": \"Find\", \"args\": {\"query\": \"test\"}}]}\n```",
						},
					},
				},
			}, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "search"}},
	}, logger)
	plan, err := strategy.Generate(context.Background(), "test task")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "search" {
		t.Errorf("expected 'search', got '%s'", plan.Steps[0].ToolName)
	}
}

func TestChatRequest_JSONMarshal(t *testing.T) {
	cfg := proxy.ChatRequest{MaxTokens: 100}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

// TestExecutionPlanStrategy_StreamFallback proves the plan-generation primitive
// falls back to the non-streaming Chat path when the provider cannot stream
// (SSE-less providers / stream-start failure) — the plan still parses. Mirrors
// computeNextResponse's streaming→non-streaming fallback; keeps the existing
// ChatFunc-driven plan-execute tests on the same path.
func TestExecutionPlanStrategy_StreamFallback(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{
						Message: proxy.Message{
							Content: `{"description": "Test", "steps": [{"tool": "read_file", "description": "Read", "args": {"path": "test.txt"}}]}`,
						},
					},
				},
			}, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
	}, logger)
	plan, err := strategy.Generate(context.Background(), "test task")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "read_file" {
		t.Errorf("expected 'read_file', got '%s'", plan.Steps[0].ToolName)
	}
}

// TestExecutionPlanStrategy_Generate_AppliesRequestConfig proves the shared
// request-config hook (Agent.applyRequestConfig) is applied to the
// plan-generation request: the request that reaches the LLM carries the
// temperature and reasoning wire params configured for the model, matching
// normal turns.
func TestExecutionPlanStrategy_Generate_AppliesRequestConfig(t *testing.T) {
	var captured proxy.ChatRequest
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			captured = req
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{
						Message: proxy.Message{
							Content: `{"description": "Test", "steps": [{"tool": "read_file", "description": "Read", "args": {"path": "test.txt"}}]}`,
						},
					},
				},
			}, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
	}, logger,
		withApplyRequest(func(req *proxy.ChatRequest) {
			req.Temperature = 0.7
			req.ThinkingBudgetTokens = 512
		}),
	)
	if _, err := strategy.Generate(context.Background(), "test task"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if captured.Temperature != 0.7 {
		t.Errorf("expected plan request temperature 0.7, got %v", captured.Temperature)
	}
	if captured.ThinkingBudgetTokens != 512 {
		t.Errorf("expected plan request thinking_budget_tokens 512, got %d", captured.ThinkingBudgetTokens)
	}
}

// TestExecutionPlanStrategy_PlanGenStream_EmitsStillThinkingOnSilentStall: a
// plan-generation stream that stalls past one heartbeat cadence must emit
// still_thinking (via the onLifecycle hook) so the UI never shows a dead bubble
// during a long plan-gen TTFT — same silent-stall gate as processStream.
func TestExecutionPlanStrategy_PlanGenStream_EmitsStillThinkingOnSilentStall(t *testing.T) {
	setStreamHeartbeatInterval(t, 5*time.Millisecond)

	var stillThinking int
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					ReasoningContent: "planning...",
				}}}}
				// Stall longer than several heartbeat ticks.
				time.Sleep(60 * time.Millisecond)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					Content: `{"description": "Test", "steps": [{"tool": "read_file", "description": "Read", "args": {"path": "test.txt"}}]}`,
				}}}}
			}()
			return ch, nil
		},
	}
	logger := logging.NewNopLogger()
	strategy := NewExecutionPlanStrategy(client, []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
	}, logger,
		withOnReasoning(func(string) {}),
		withOnLifecycle(func(phase string, extra map[string]any) {
			if phase == PhaseStillThinking {
				stillThinking++
			}
		}),
	)
	plan, err := strategy.Generate(context.Background(), "test task")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if stillThinking == 0 {
		t.Error("expected still_thinking during silent plan-gen stall, got none")
	}
}
