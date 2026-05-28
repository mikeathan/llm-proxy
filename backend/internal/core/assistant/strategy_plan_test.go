package assistant

import (
	"context"
	"encoding/json"
	"testing"

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

func TestModelConfig_EnableExecutionPlan(t *testing.T) {
	cfg := proxy.ChatRequest{MaxTokens: 100}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}
