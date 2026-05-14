package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"strings"
	"testing"
	"time"
)

// MockClient implements proxy.Client
type MockClient struct {
	Response proxy.ChatResponse
	Err      error
	Calls    int
	ChatFunc func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error)
	StreamFunc func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error)
}

func (m *MockClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, req)
	}
	// Fall back to non-streaming by default for existing tests
	return nil, fmt.Errorf("streaming not implemented in mock")
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

func (m *MockProvider) UseNativeTools() bool {
	return true
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
				{Message: proxy.Message{Role: "assistant", Content: "# Summary\nHello world"}},
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
	if reply != "# Summary\nHello world" {
		t.Errorf("Expected '# Summary\nHello world', got '%s'", reply)
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
				{Message: proxy.Message{Role: "assistant", Content: "# Summary\nIt is sunny in London."}},
			},
		}, nil
	}

	reply, history, err := agent.Execute(ctx, []proxy.Message{{Role: "user", Content: "Weather in London?"}})
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Summary\nIt is sunny in London." {
		t.Errorf("Got unexpected reply: %s", reply)
	}
	if client.Calls != 2 {
		t.Errorf("Expected 2 client calls, got %d", client.Calls)
	}
	if len(history) != 4 { // user + assistant (tc) + tool result + assistant (final)
		t.Errorf("Expected history length 4, got %d", len(history))
	}
}

func TestAgent_Execute_LoopDetection(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_loop",
								Function: proxy.FunctionCall{
									Name:      "ping",
									Arguments: `{}`,
								},
							},
						},
					},
				},
			},
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "ping"}}},
	}
	engine := &MockEngine{Result: "pong"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 10})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "loop please"}})

	if err == nil || !strings.Contains(err.Error(), "infinite loop detected") {
		t.Fatalf("Expected infinite loop error, got: %v", err)
	}
}

func TestAgent_Execute_TotalTimeout(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "Too slow"}}},
				}, nil
			}
		},
	}

	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})

	// Create a context that will expire quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := agent.Execute(ctx, []proxy.Message{{Role: "user", Content: "Wait"}})

	if err == nil || (!strings.Contains(err.Error(), "halted") && !strings.Contains(err.Error(), "context deadline exceeded")) {
		t.Fatalf("Expected timeout error, got: %v", err)
	}
}

func TestAgent_IsPrematureTermination(t *testing.T) {
	agent := &Agent{}
	tests := []struct {
		name     string
		content  string
		history  []proxy.Message
		expected bool
	}{
		{"empty", "", nil, true},
		{"short but valid", "Hello.", []proxy.Message{{Role: proxy.AssistantRole, Content: "Hello."}}, false},
		{"long valid", strings.Repeat("a", 100), []proxy.Message{{Role: proxy.AssistantRole, Content: strings.Repeat("a", 100)}}, false},
		{"repetition", "Thinking...", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Thinking..."},
			{Role: proxy.AssistantRole, Content: "Thinking..."},
		}, true},
		{"not repetition (different content)", "New thought.", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Old thought."},
			{Role: proxy.AssistantRole, Content: "New thought."},
		}, false},
		{"not repetition (intervening tool)", "Thinking...", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Thinking..."},
			{Role: proxy.ToolRole, Content: "result"},
			{Role: proxy.AssistantRole, Content: "Thinking..."},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := proxy.Message{Content: tt.content}
			got := agent.isPrematureTermination(msg, tt.history)
			if got != tt.expected {
				t.Errorf("isPrematureTermination(%q) = %v, want %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestAgent_Execute_StreamWithXMLToolCall(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 3)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Let me check "}}}}
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "the file.\n"}}}}
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "read_file", "args": {"path": "test.txt"}}
</tool_call>`}}}}
			}()
			return ch, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "file contents here"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	_, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Read test.txt"}})

	if err != nil && !strings.Contains(err.Error(), "infinite loop") {
		t.Fatalf("Execute failed: %v", err)
	}
	// Should have detected the embedded XML tool call and executed it
	foundTool := false
	for _, m := range history {
		if m.Role == proxy.ToolRole {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Error("expected tool execution result in history")
	}
}

func TestAgent_Execute_StreamEmptyFallback(t *testing.T) {
	// Stream returns empty content — should fall back to non-streaming.
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
				// Empty stream — no chunks
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "Fallback response"}}},
			}, nil
		},
	}

	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Hello"}})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "Fallback response" {
		t.Errorf("expected 'Fallback response', got '%s'", reply)
	}
	if callCount < 1 {
		t.Error("expected non-streaming fallback to be called")
	}
}

func TestAgent_Execute_StreamWithInterleavedToolCalls(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 3)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "I'll read the file.\n"}}}}
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "read_file", "args": {"path": "/etc/hosts"}}
</tool_call>`}}}}
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "\nDone."}}}}
			}()
			return ch, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "127.0.0.1 localhost"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	_, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Read /etc/hosts"}})

	if err != nil && !strings.Contains(err.Error(), "infinite loop") {
		t.Fatalf("Execute failed: %v", err)
	}
	foundTool := false
	for _, m := range history {
		if m.Role == proxy.ToolRole {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Error("expected tool execution from interleaved stream content")
	}
}

func TestAgent_PrecededByToolResult(t *testing.T) {
	agent := &Agent{}
	tests := []struct {
		name     string
		history  []proxy.Message
		expected bool
	}{
		{
			"empty history",
			[]proxy.Message{},
			false,
		},
		{
			"single message",
			[]proxy.Message{{Role: proxy.AssistantRole, Content: "Hello"}},
			false,
		},
		{
			"assistant follows tool result",
			[]proxy.Message{
				{Role: proxy.ToolRole, Content: `"result data"`},
				{Role: proxy.AssistantRole, Content: "Got the result, here's what I found."},
			},
			true,
		},
		{
			"assistant follows assistant (no tool in between)",
			[]proxy.Message{
				{Role: proxy.AssistantRole, Content: "First reply"},
				{Role: proxy.AssistantRole, Content: "Second reply"},
			},
			false,
		},
		{
			"assistant with empty content after tool",
			[]proxy.Message{
				{Role: proxy.ToolRole, Content: `"result"`},
				{Role: proxy.AssistantRole, Content: ""},
			},
			false,
		},
		{
			"user message after tool (not assistant)",
			[]proxy.Message{
				{Role: proxy.ToolRole, Content: `"result"`},
				{Role: proxy.UserRole, Content: "What next?"},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.precededByToolResult(tt.history)
			if got != tt.expected {
				t.Errorf("precededByToolResult() = %v, want %v", got, tt.expected)
			}
		})
	}
}
