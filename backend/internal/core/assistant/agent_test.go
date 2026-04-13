package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"testing"
)

// MockClient implements proxy.Client
type MockClient struct {
	Response proxy.ChatResponse
	Err      error
	Calls    int
	ChatFunc func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error)
}

func (m *MockClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	m.Calls++
	if m.ChatFunc != nil {
		return m.ChatFunc(ctx, req)
	}
	return &m.Response, m.Err
}

// MockProvider implements ToolProvider
type MockProvider struct {
	Tools []proxy.Tool
}

func (m *MockProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	return m.Tools, nil
}

func (m *MockProvider) GetSystemPrompt() (string, error) {
	return "test prompt", nil
}

// MockEngine implements Engine
type MockEngine struct {
	Result any
	Err    error
}

func (m *MockEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	return m.Result, m.Err
}

func TestAgent_Execute_Simple(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "Hello world"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	
	reply, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Hi"}})
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "Hello world" {
		t.Errorf("Expected 'Hello world', got '%s'", reply)
	}
	if len(history) != 2 { // user + assistant
		t.Errorf("Expected history length 2, got %d", len(history))
	}
	if client.Calls != 1 {
		t.Errorf("Expected 1 client call, got %d", client.Calls)
	}
}

func TestAgent_Execute_ToolCall(t *testing.T) {
	// 1. First call returns a tool call
	// 2. Second call returns final content
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_123",
								Function: proxy.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"city": "London"}`,
								},
							},
						},
					},
				},
			},
		},
	}
	
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "get_weather"}},
		},
	}
	engine := &MockEngine{Result: "Sunny"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	
	// Override client behavior for second call
	ctx := context.Background()
	client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		if client.Calls == 1 {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{
						Message: proxy.Message{
							Role: "assistant",
							ToolCalls: []proxy.ToolCall{
								{
									ID: "call_123",
									Function: proxy.FunctionCall{
										Name:      "get_weather",
										Arguments: `{"city": "London"}`,
									},
								},
							},
						},
					},
				},
			}, nil
		}
		// Check that tool result was appended
		if len(req.Messages) < 3 {
			return nil, fmt.Errorf("expected at least 3 messages in second call")
		}
		toolMsg := req.Messages[len(req.Messages)-1]
		if toolMsg.Role != proxy.ToolRole || toolMsg.Content != `"Sunny"` {
			return nil, fmt.Errorf("tool result not found in history: %v", toolMsg)
		}
		
		return &proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "It is sunny in London."}},
			},
		}, nil
	}

	reply, history, err := agent.Execute(ctx, []proxy.Message{{Role: "user", Content: "Weather in London?"}})
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "It is sunny in London." {
		t.Errorf("Got unexpected reply: %s", reply)
	}
	if client.Calls != 2 {
		t.Errorf("Expected 2 client calls, got %d", client.Calls)
	}
	if len(history) != 4 { // user + assistant (tc) + tool result + assistant (final)
		t.Errorf("Expected history length 4, got %d", len(history))
	}
}
