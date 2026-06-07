package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"strings"
	"sync"
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
	Tools         []proxy.Tool
	UseNative     *bool  // nil = true (backward compat)
}

func (m *MockProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	return m.Tools, nil
}

func (m *MockProvider) GetSystemPrompt() (string, error) {
	return "test prompt", nil
}

func (m *MockProvider) UseNativeTools() bool {
	if m.UseNative == nil {
		return true
	}
	return *m.UseNative
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

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      10,
		GlobalTimeout: 500 * time.Millisecond,
	})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "loop please"}})

	if err == nil {
		t.Fatalf("Expected an error (timeout or loop), got nil")
	}
	t.Logf("Loop detection exited with: %v", err)
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
		name             string
		content          string
		history          []proxy.Message
		expected         bool
		reasoningContent string
	}{
		{"empty", "", nil, true, ""},
		{"short but valid", "Hello.", []proxy.Message{{Role: proxy.AssistantRole, Content: "Hello."}}, false, ""},
		{"long valid", strings.Repeat("a", 100), []proxy.Message{{Role: proxy.AssistantRole, Content: strings.Repeat("a", 100)}}, false, ""},
		{"repetition", "Thinking...", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Thinking..."},
			{Role: proxy.AssistantRole, Content: "Thinking..."},
		}, true, ""},
		{"not repetition (different content)", "New thought.", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Old thought."},
			{Role: proxy.AssistantRole, Content: "New thought."},
		}, false, ""},
		{"not repetition (intervening tool)", "Thinking...", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Thinking..."},
			{Role: proxy.ToolRole, Content: "result"},
			{Role: proxy.AssistantRole, Content: "Thinking..."},
		}, false, ""},
		{"reasoning only (not premature)", "", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "Previous step"},
		}, false, "Thinking about what to do next"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := proxy.Message{Content: tt.content, ReasoningContent: tt.reasoningContent}
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

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 2 * time.Second,
	})
	_, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Read test.txt"}})

	if err != nil && !strings.Contains(err.Error(), "infinite loop") && !strings.Contains(err.Error(), "halted") && !strings.Contains(err.Error(), "deadline") {
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

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 2 * time.Second,
	})
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

func TestAgent_Execute_ReasoningStuckFallback(t *testing.T) {
	// Stream returns only reasoning content (no text, no tool calls above threshold)
	// — should trigger the reactive sieve and retry.
	streamCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCount++
			ch := make(chan *proxy.ChatResponse, 100)
			go func() {
				defer close(ch)
				for i := 0; i < 250; i++ {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
						Delta: proxy.Message{ReasoningContent: "the model keeps thinking without producing a tool call or text "},
					}}}
				}
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			// After the sieve retries, eventually produce a tool call
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_submit",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "Task complete"}`,
							},
						}},
					},
				}},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 20, GlobalTimeout: 2 * time.Second})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	// Under the new fallback mechanism, the reasoning stuck stream abort
	// triggers the non-streaming fallback, which succeeds.
	if err != nil {
		t.Fatalf("expected successful recovery via non-streaming fallback, got error: %v", err)
	}
	if !strings.Contains(reply, "Task complete") {
		t.Errorf("expected final answer reply, got '%s'", reply)
	}
	if streamCount < 1 {
		t.Errorf("expected stream to be attempted at least once, got %d", streamCount)
	}
}

func TestAgent_Execute_NativeToolsEmptyStreamFallsBackToXML(t *testing.T) {
	toolsPassed := false
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			if len(req.Tools) > 0 {
				toolsPassed = true
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_submit",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "Task complete"}`,
							},
						}},
					},
				}},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 2 * time.Second,
	})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if toolsPassed {
		t.Error("non-streaming fallback should NOT receive native tools — should retry with XML text mode")
	}
	if !agent.useNativeTools {
		t.Error("agent should restore native tools after temporary fallback override")
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

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 2 * time.Second,
	})
	_, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Read /etc/hosts"}})

	if err != nil && !strings.Contains(err.Error(), "infinite loop") && !strings.Contains(err.Error(), "halted") && !strings.Contains(err.Error(), "deadline") {
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

func TestIsToolCallParseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "empty error",
			err:  fmt.Errorf(""),
			want: false,
		},
		{
			name: "arbitrary error",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
		{
			name: "llama.cpp tool call parse error (stream)",
			err:  fmt.Errorf(`llm completion failed: LLM stream error 500: {"error":{"code":500,"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error at line 1, column 730: syntax error while parsing value - invalid string: missing closing quote"}}`),
			want: true,
		},
		{
			name: "llama.cpp tool call parse error (non-stream)",
			err:  fmt.Errorf(`llm completion failed: LLM chat error 500: {"error":{"code":500,"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error"}}`),
			want: true,
		},
		{
			name: "wrapped error with tool call parse",
			err:  fmt.Errorf("something went wrong: LLM chat error 500: {\"error\":{\"message\":\"Failed to parse tool call arguments as JSON: unexpected end of input\"}}"),
			want: true,
		},
		{
			name: "context size error should not match",
			err:  fmt.Errorf("llm completion failed: context size exceeded"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isToolCallParseError(tt.err)
			if got != tt.want {
				t.Errorf("isToolCallParseError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgent_ToolCallParseErrorRetry(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf(`llm completion failed: LLM chat error 500: {"error":{"code":500,"message":"Failed to parse tool call arguments as JSON: [json.exception.parse_error.101] parse error at line 1, column 730: syntax error while parsing value - invalid string: missing closing quote"}}`)
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    "assistant",
						Content: "Here is the final result.",
						ToolCalls: []proxy.ToolCall{{
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "Report written successfully"}`,
							},
						}},
					}},
				},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "Task submitted successfully."}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 2 * time.Second,
	})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Write a large report"}})

	if err != nil {
		t.Fatalf("Execute should have recovered from tool call parse error, got: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (1 error + 1 retry), got %d", callCount)
	}
	if !strings.Contains(reply, "Report written successfully") {
		t.Errorf("expected final answer summary, got '%s'", reply)
	}
}

func TestAgent_ToolCallParseErrorExhaustRetries(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return nil, fmt.Errorf(`llm completion failed: LLM chat error 500: {"error":{"code":500,"message":"Failed to parse tool call arguments as JSON: truncated"}}`)
		},
	}

	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      10,
		GlobalTimeout: 2 * time.Second,
	})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Write a large report"}})

	if err == nil {
		t.Fatal("expected error after exhausting retries with persistent tool call parse errors")
	}
	if !strings.Contains(err.Error(), "stalled") {
		t.Errorf("expected stalled agent error, got: %v", err)
	}
}

func TestIsPrefillThinkingError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no prefill", fmt.Errorf("connection reset"), false},
		{"prefill but no thinking", fmt.Errorf("prefill failed: token limit"), false},
		{"prefill + thinking (lowercase)", fmt.Errorf("prefill with thinking mode not supported"), true},
		{"prefill + thinking (mixed)", fmt.Errorf("prefill rejected: Thinking mode active"), true},
		{"prefill + thinking (uppercase)", fmt.Errorf("PREFILL THINKING ERROR"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPrefillThinkingError(tt.err)
			if got != tt.want {
				t.Errorf("isPrefillThinkingError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsToolSupportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"arbitrary", fmt.Errorf("connection refused"), false},
		{"tools not supported", fmt.Errorf("parameter `tools` is not currently supported"), true},
		{"tool choice not supported", fmt.Errorf("tool_choice is not supported with this model"), true},
		{"auto tool choice", fmt.Errorf("auto tool choice requires function calling"), true},
		{"tools in params", fmt.Errorf("parameter `tools` is not allowed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isToolSupportError(tt.err)
			if got != tt.want {
				t.Errorf("isToolSupportError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsContextSizeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"arbitrary", fmt.Errorf("connection refused"), false},
		{"context size", fmt.Errorf("request exceeds the available context size"), true},
		{"context length exceeded", fmt.Errorf("context_length_exceeded"), true},
		{"max context length", fmt.Errorf("maximum context length exceeded"), true},
		{"reduce the length", fmt.Errorf("please reduce the length of the messages"), true},
		{"too many tokens", fmt.Errorf("too many tokens for this model"), true},
		{"token limit", fmt.Errorf("token limit reached"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isContextSizeError(tt.err)
			if got != tt.want {
				t.Errorf("isContextSizeError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTruncationError(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want bool
	}{
		{"empty", "", false},
		{"arbitrary", "something went wrong", false},
		{"unexpected end", "unexpected end of JSON input", true},
		{"missing closing", "missing closing quote", true},
		{"unexpected end in long message", "parse error: unexpected end of input while parsing", true},
		{"missing closing in wrapped", "wrapper: missing closing delimiter", true},
		{"no match", "invalid character", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTruncationError(tt.err)
			if got != tt.want {
				t.Errorf("isTruncationError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestParseErrorKind(t *testing.T) {
	tests := []struct {
		name string
		pe   *proxy.ParseError
		want string
	}{
		{"nil", nil, ""},
		{"no xml", &proxy.ParseError{XMLFound: false, JSONError: "", ToolName: ""}, "no_xml"},
		{"json error", &proxy.ParseError{XMLFound: true, JSONError: "invalid json", ToolName: ""}, "json"},
		{"tool name", &proxy.ParseError{XMLFound: true, JSONError: "", ToolName: "bad_tool"}, "tool_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseErrorKind(tt.pe)
			if got != tt.want {
				t.Errorf("parseErrorKind() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountConsecutiveChat(t *testing.T) {
	agent := &Agent{}
	tests := []struct {
		name    string
		history []proxy.Message
		want    int
	}{
		{"empty", []proxy.Message{}, 0},
		{"single assistant", []proxy.Message{{Role: proxy.AssistantRole, Content: "Hi"}}, 1},
		{"two assistants", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "A"},
			{Role: proxy.AssistantRole, Content: "B"},
		}, 2},
		{"assistant then user", []proxy.Message{
			{Role: proxy.AssistantRole, Content: "A"},
			{Role: proxy.UserRole, Content: "B"},
		}, 0},
		{"tool then assistant", []proxy.Message{
			{Role: proxy.ToolRole, Content: "result"},
			{Role: proxy.AssistantRole, Content: "done"},
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.countConsecutiveChat(tt.history)
			if got != tt.want {
				t.Errorf("countConsecutiveChat() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAgent_ContextSizeErrorRecovery(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("llm completion failed: request exceeds the available context size")
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_submit",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "Recovered after sieve."}`,
							},
						}},
					}},
				},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})

	// Create initial history with enough messages to test sieve
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: "task: do something"},
	}
	for i := 0; i < 15; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: fmt.Sprintf("message %d", i)})
	}

	reply, _, err := agent.Execute(context.Background(), history)

	if err != nil {
		t.Fatalf("Execute should have recovered from context size error, got: %v", err)
	}
	if !strings.Contains(reply, "Recovered after sieve") {
		t.Errorf("expected recovered response, got '%s'", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (1 error + 1 retry), got %d", callCount)
	}
}

func TestAgent_ToolSupportFallback(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("llm completion failed: parameter `tools` is not currently supported")
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "Fallback without tools."}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Hi"}})

	if err != nil {
		t.Fatalf("Execute should have recovered from tool support error, got: %v", err)
	}
	if reply != "Fallback without tools." {
		t.Errorf("expected fallback response, got '%s'", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (1 error + 1 retry), got %d", callCount)
	}
}

func TestAgent_ParseErrorStreakNotification(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			// Return plain content that fails XML parse, simulating
			// a model that keeps producing non-XML output.
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "I do not know about tools."}},
				},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 12})

	// Track notifications by capturing the observer
	var events []AgentEvent
	agent.observer = func(ev AgentEvent) {
		events = append(events, ev)
	}

	// Use automation marker so the agent doesn't exit on first step
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " read the file"},
	})

	if err == nil {
		t.Fatal("expected error after exhausting retries with persistent parse errors")
	}
	if !strings.Contains(err.Error(), "stalled") {
		t.Errorf("expected stalled agent error, got: %v", err)
	}

	// Should have received a model compat warning from the streak
	compatWarnings := 0
	for _, ev := range events {
		if ev.Type == EventMessage {
			if msg, ok := ev.Payload.(proxy.Message); ok && strings.Contains(msg.Content, "model is not generating valid tool calls") {
				compatWarnings++
			}
		}
	}
	if compatWarnings == 0 {
		t.Error("expected at least one model compat warning notification")
	}
}

func TestAgent_PrematureTerminationInAutomation(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "Chatting instead of using tools."}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})

	// Trigger automation mode via the automation marker in user content
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " some task"},
	}

	_, _, err := agent.Execute(context.Background(), history)

	if err == nil {
		t.Fatal("expected stalled agent error in automation mode without tool calls")
	}
	if !strings.Contains(err.Error(), "stalled") {
		t.Errorf("expected stalled agent error, got: %v", err)
	}
}

func TestAgent_NonAutomationMultipleSteps(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{
						{Message: proxy.Message{
							Role: "assistant",
							ToolCalls: []proxy.ToolCall{{
								ID:       "call_1",
								Function: proxy.FunctionCall{Name: "get_weather", Arguments: `{"city":"London"}`},
							}},
						}},
					},
				}, nil
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "It is sunny."}},
				},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "get_weather"}},
		},
	}
	engine := &MockEngine{Result: `"Sunny"`}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Weather?"}})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "It is sunny." {
		t.Errorf("expected final response, got '%s'", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}
}

func TestNotifyPrefillDisabled(t *testing.T) {
	agent := &Agent{}
	var events []AgentEvent
	agent.observer = func(ev AgentEvent) { events = append(events, ev) }

	agent.notifyPrefillDisabled()

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventMessage {
		t.Errorf("expected EventMessage, got %s", events[0].Type)
	}
	msg, ok := events[0].Payload.(proxy.Message)
	if !ok {
		t.Fatal("expected proxy.Message payload")
	}
	if !strings.Contains(msg.Content, "prefill") {
		t.Errorf("expected prefill-related content, got '%s'", msg.Content)
	}
}

func TestNotifyModelCompatWarning(t *testing.T) {
	tests := []struct {
		name           string
		useNativeTools bool
		wantSuggest    string
	}{
		{"native tools failing → suggest xml", true, "xml"},
		{"xml tools failing → suggest native", false, "native"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &Agent{}
			var events []AgentEvent
			agent.observer = func(ev AgentEvent) { events = append(events, ev) }

			agent.notifyModelCompatWarning(tt.useNativeTools)

			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			msg, ok := events[0].Payload.(proxy.Message)
			if !ok {
				t.Fatal("expected proxy.Message payload")
			}
			if !strings.Contains(msg.Content, tt.wantSuggest) {
				t.Errorf("expected suggestion to contain '%s', got '%s'", tt.wantSuggest, msg.Content)
			}
		})
	}
}

func TestAgent_PhysicalContextSieve(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "# Summary\nDone"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		ContextBudget: 100,
	})

	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "System: You are an assistant."},
		{Role: proxy.UserRole, Content: "Task: write code"},
	}
	for i := 0; i < 12; i++ {
		history = append(history, proxy.Message{
			Role:    proxy.AssistantRole,
			Content: fmt.Sprintf("Intermediate message content that is relatively long %d", i),
		})
	}

	_, finalHistory, err := agent.Execute(context.Background(), history)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	foundSieveNote := false
	foundSieveWarning := false
	for _, msg := range finalHistory {
		if strings.Contains(msg.Content, prompts.SieveSystemNote) {
			foundSieveNote = true
		}
		if strings.Contains(msg.Content, prompts.ContextSieveWarning) {
			foundSieveWarning = true
		}
	}

	if !foundSieveNote {
		t.Error("Expected prompts.SieveSystemNote in sieved history")
	}
	if !foundSieveWarning {
		t.Error("Expected prompts.ContextSieveWarning in sieved history")
	}
}

func TestAgent_ValidateToolArgs(t *testing.T) {
	tools := []proxy.Tool{
		{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name:        "test_tool",
				Description: "A test tool",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"req_param": map[string]any{"type": "string"},
						"opt_param": map[string]any{"type": "string"},
					},
					"required": []any{"req_param"},
				},
			},
		},
	}

	tests := []struct {
		name    string
		tc      proxy.ToolCall
		wantErr string
	}{
		{
			name: "valid args",
			tc: proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"req_param": "hello"}`,
				},
			},
			wantErr: "",
		},
		{
			name: "missing required",
			tc: proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"opt_param": "hello"}`,
				},
			},
			wantErr: "missing required parameter 'req_param'",
		},
		{
			name: "empty required",
			tc: proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"req_param": "  "}`,
				},
			},
			wantErr: "parameter 'req_param' cannot be empty",
		},
		{
			name: "invalid json",
			tc: proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"req_param": "hello"`,
				},
			},
			wantErr: "failed to parse arguments as JSON",
		},
		{
			name: "unknown tool",
			tc: proxy.ToolCall{
				Function: proxy.FunctionCall{
					Name:      "unknown_tool",
					Arguments: `{}`,
				},
			},
			wantErr: "tool 'unknown_tool' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToolArgs(tt.tc, tools)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestAgent_Execute_BatchedSubmissionRejection(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_submit",
								Function: proxy.FunctionCall{
									Name:      models.ToolSubmitFinalAnswer,
									Arguments: `{"summary": "Task complete"}`,
								},
							},
							{
								ID: "call_other",
								Function: proxy.FunctionCall{
									Name:      "read_file",
									Arguments: `{"path": "test.txt"}`,
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
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{}
	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})

	client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		return &client.Response, nil
	}

	_, finalHistory, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Run task"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	foundRejectionError := 0
	for _, msg := range finalHistory {
		if msg.Role == proxy.ToolRole && strings.Contains(msg.Content, prompts.AutomationRejectedSubmissionPrompt) {
			foundRejectionError++
		}
	}
	if foundRejectionError != 2 {
		t.Errorf("expected 2 rejection error results in final history, got %d", foundRejectionError)
	}
}

func TestAgent_Execute_GuardrailDecisionApproval(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_blocked",
								Function: proxy.FunctionCall{
									Name:      "test_tool",
									Arguments: `{"secret": "sk-12345678901234567890123456789012"}`,
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		},
	}
	engine := &MockEngine{Result: "success"}

	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			Global: models.GlobalGuardrailsConfig{
				BlockSecrets: true,
			},
		}
	}, storage.NewPathResolver("", "", ""), nil)

	var callbackPayload GuardrailBlockedPayload
	var callbackCalled bool

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:   5,
		Guardrails: gr,
		GuardrailDecisionHandler: func(ctx context.Context, p GuardrailBlockedPayload) (GuardrailDecision, error) {
			callbackCalled = true
			callbackPayload = p
			return GuardrailDecision{Allow: true, Persist: false}, nil
		},
	})

	client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		if client.Calls == 1 {
			return &client.Response, nil
		}
		return &proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_submit",
								Function: proxy.FunctionCall{
									Name:      models.ToolSubmitFinalAnswer,
									Arguments: `{"summary": "Succeeded"}`,
								},
							},
						},
					},
				},
			},
		}, nil
	}

	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Run with secret"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Expected guardrail decision callback to be called")
	}
	if callbackPayload.Tool != "test_tool" {
		t.Errorf("Expected tool test_tool, got %s", callbackPayload.Tool)
	}
}

func TestAgent_Execute_GuardrailDecisionDenial(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_blocked",
								Function: proxy.FunctionCall{
									Name:      "test_tool",
									Arguments: `{"secret": "sk-12345678901234567890123456789012"}`,
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		},
	}
	engine := &MockEngine{Result: "success"}

	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			Global: models.GlobalGuardrailsConfig{
				BlockSecrets: true,
			},
		}
	}, storage.NewPathResolver("", "", ""), nil)

	var callbackCalled bool

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:   5,
		Guardrails: gr,
		GuardrailDecisionHandler: func(ctx context.Context, p GuardrailBlockedPayload) (GuardrailDecision, error) {
			callbackCalled = true
			return GuardrailDecision{Allow: false, Persist: false}, nil
		},
	})

	var secondCallReq *proxy.ChatRequest
	client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		if client.Calls == 1 {
			return &client.Response, nil
		}
		secondCallReq = &req
		return &proxy.ChatResponse{
			Choices: []proxy.Choice{
				{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID: "call_submit",
								Function: proxy.FunctionCall{
									Name:      models.ToolSubmitFinalAnswer,
									Arguments: `{"summary": "Denied"}`,
								},
							},
						},
					},
				},
			},
		}, nil
	}

	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Run with secret"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Expected guardrail decision callback to be called")
	}
	if secondCallReq == nil {
		t.Fatal("Expected second call request to be captured")
	}

	foundViolationError := false
	for _, msg := range secondCallReq.Messages {
		if msg.Role == proxy.ToolRole && strings.Contains(msg.Content, "Guardrail violation") {
			foundViolationError = true
			break
		}
	}
	if !foundViolationError {
		t.Error("Expected guardrail violation error message in the next request's history")
	}
}

func TestAgent_NativeToolsNoXMLManualInAutomation(t *testing.T) {
	manualCheckDone := false
	foundNativeRef := false
	streamCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			if !manualCheckDone {
				manualCheckDone = true
				for _, m := range req.Messages {
					if strings.Contains(m.Content, "TOOL INTERFACE") {
						t.Error("XML TOOL INTERFACE manual should NOT be in messages when native tools are enabled")
					}
					if strings.Contains(m.Content, prompts.ToolReferenceHeader) {
						foundNativeRef = true
					}
				}
			}
			streamCount++
			ch := make(chan *proxy.ChatResponse, 3)
			go func() {
				defer close(ch)
				if streamCount == 1 {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Let me read the file.\n"}}}}
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "read_file", "args": {"path": "test.txt"}}
</tool_call>`}}}}
				} else {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "File read.\n"}}}}
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "submit_final_answer", "args": {"summary": "Task complete"}}
</tool_call>`}}}}
				}
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID: "call_submit",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "Task complete"}`,
							},
						}},
					}},
				},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: `"file contents"`}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " read test.txt and answer"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !manualCheckDone {
		t.Error("manual check for TOOL INTERFACE was never performed")
	}
	if !foundNativeRef {
		t.Error("expected AVAILABLE TOOLS native reference in messages when native tools are enabled")
	}
}

func TestAgent_Execute_NativeToolsSetsToolChoice(t *testing.T) {
	var capturedReq proxy.ChatRequest
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			capturedReq = req
			ch := make(chan *proxy.ChatResponse, 2)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "submit_final_answer", "args": {"summary": "Done"}}
</tool_call>`}}}}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 3})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " run test"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if capturedReq.ToolChoice != proxy.ToolChoiceRequired {
		t.Errorf("expected ToolChoice=%q, got %q", proxy.ToolChoiceRequired, capturedReq.ToolChoice)
	}
	if capturedReq.Temperature != 0.1 {
		t.Errorf("expected Temperature=0.1 for automation, got %f", capturedReq.Temperature)
	}
	if capturedReq.ReasoningBudget == 0 {
		t.Errorf("expected non-zero ReasoningBudget for native tools+automation, got %d", capturedReq.ReasoningBudget)
	}
}

func TestAgent_Execute_NativeToolsTemperatureSet(t *testing.T) {
	var capturedReq proxy.ChatRequest
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			capturedReq = req
			ch := make(chan *proxy.ChatResponse, 2)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "submit_final_answer", "args": {"summary": "Done"}}
</tool_call>`}}}}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 3})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " run test"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if capturedReq.Temperature != 0.1 {
		t.Errorf("expected Temperature=0.1 for automation, got %f", capturedReq.Temperature)
	}
}

func TestAgent_Execute_XMLToolChoiceUnset(t *testing.T) {
	var capturedReq proxy.ChatRequest
	falseVal := false
	client := &MockClient{
		// Fall back to non-streaming so prefill doesn't corrupt the capture
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			capturedReq = req
			ch := make(chan *proxy.ChatResponse, 2)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "submit_final_answer", "args": {"summary": "Done"}}
</tool_call>`}}}}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       5,
		UseNativeTools: &falseVal,
	})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " run test"},
	})
	if err != nil && !strings.Contains(err.Error(), "max steps") && !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("Execute failed: %v", err)
	}
	if capturedReq.ToolChoice != "" {
		t.Errorf("expected empty ToolChoice for XML mode, got %q", capturedReq.ToolChoice)
	}
	if capturedReq.Temperature != 0.1 {
		t.Errorf("expected Temperature=0.1 for XML mode (automation), got %f", capturedReq.Temperature)
	}
	if capturedReq.ReasoningBudget == 0 {
		t.Errorf("expected non-zero ReasoningBudget for XML mode (automation), got %d", capturedReq.ReasoningBudget)
	}
}

func TestRepetitionDetector_StreakReset(t *testing.T) {
	logger := logging.NewNopLogger()
	rd := repetitionDetector{}

	// Call tool A
	toolCallsA := []proxy.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: proxy.FunctionCall{
				Name:      "toolA",
				Arguments: `{"arg": 1}`,
			},
		},
	}
	isDup, nag, err := rd.check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("expected first call to tool A not to be duplicate")
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak to be 0, got %d", rd.duplicateStreak)
	}

	// Call tool A again -> duplicate
	isDup, nag, err = rd.check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDup {
		t.Error("expected second consecutive call to tool A to be duplicate")
	}
	if nag != prompts.AutomationDuplicateNagPrompt {
		t.Errorf("expected nag prompt, got %q", nag)
	}
	if rd.duplicateStreak != 1 {
		t.Errorf("expected duplicateStreak to be 1, got %d", rd.duplicateStreak)
	}

	// Call tool B -> resets streak
	toolCallsB := []proxy.ToolCall{
		{
			ID:   "2",
			Type: "function",
			Function: proxy.FunctionCall{
				Name:      "toolB",
				Arguments: `{"arg": 2}`,
			},
		},
	}
	isDup, nag, err = rd.check(logger, toolCallsB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("expected call to tool B not to be duplicate")
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak to reset to 0, got %d", rd.duplicateStreak)
	}

	// Call tool A again -> found but NOT consecutive (B is last), allowed to execute
	isDup, nag, err = rd.check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDup {
		t.Error("expected tool A call after tool B to be allowed (non-consecutive)")
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak to be 0, got %d", rd.duplicateStreak)
	}
	if nag != "" {
		t.Errorf("expected empty nag on non-consecutive duplicate, got %q", nag)
	}

	// Call tool A again -> consecutive duplicate (last key is A), streak = 1
	isDup, _, _ = rd.check(logger, toolCallsA)
	if !isDup || rd.duplicateStreak != 1 {
		t.Errorf("expected consecutive duplicate, got isDup=%t streak=%d", isDup, rd.duplicateStreak)
	}

	// Call tool A again -> consecutive duplicate, streak = 2
	isDup, _, err = rd.check(logger, toolCallsA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDup || rd.duplicateStreak != 2 {
		t.Errorf("expected duplicate on second consecutive, got isDup=%t streak=%d", isDup, rd.duplicateStreak)
	}

	// Call tool A again -> consecutive duplicate (streak = 3 -> fatal error)
	isDup, nag, err = rd.check(logger, toolCallsA)
	if err == nil {
		t.Fatal("expected error on third consecutive duplicate, got nil")
	}
	if !strings.Contains(err.Error(), "infinite loop") {
		t.Errorf("expected infinite loop error, got: %v", err)
	}
	if !isDup {
		t.Error("expected third consecutive duplicate to be detected")
	}
	if nag != "" {
		t.Errorf("expected empty nag on fatal duplicate, got %q", nag)
	}
	if rd.duplicateStreak != 0 {
		t.Errorf("expected duplicateStreak reset to 0 after skip, got %d", rd.duplicateStreak)
	}
	if rd.recentCalls != nil {
		t.Errorf("expected recentCalls cleared after skip, got %v", rd.recentCalls)
	}
}

func TestRepetitionDetector_SlidingWindow(t *testing.T) {
	logger := logging.NewNopLogger()

	makeCall := func(name, args string) proxy.ToolCall {
		return proxy.ToolCall{
			ID: fmt.Sprintf("id-%s", name), Type: "function",
			Function: proxy.FunctionCall{Name: name, Arguments: args},
		}
	}

	// Scenario 1: Consecutive duplicate detection
	// The detector only checks if the current call matches the immediately
	// previous call (consecutive duplicate).  Non-consecutive duplicates
	// reset the streak.  A→A→A hits streak=2 on the 3rd call and
	// A→A→A→A hits streak=3 → fatal on the 4th call.
	t.Run("consecutive detection", func(t *testing.T) {
		rd := repetitionDetector{}

		// First A: not a duplicate
		isDup, _, err := rd.check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isDup {
			t.Error("expected first A not duplicate")
		}

		// Second A: consecutive duplicate, streak=1
		isDup, _, err = rd.check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isDup {
			t.Error("expected second A to be duplicate (consecutive)")
		}
		if rd.duplicateStreak != 1 {
			t.Errorf("expected streak=1 after second A, got %d", rd.duplicateStreak)
		}

		// Third A: consecutive duplicate, streak=2 (still nag, not fatal)
		isDup, _, err = rd.check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isDup {
			t.Error("expected third A to be duplicate")
		}
		if rd.duplicateStreak != 2 {
			t.Errorf("expected streak=2 after third A, got %d", rd.duplicateStreak)
		}

		// Fourth A: consecutive duplicate, streak=3 → fatal error
		isDup, nag, err := rd.check(logger, []proxy.ToolCall{makeCall("toolA", `{"arg":1}`)})
		if err == nil {
			t.Fatal("expected error on 4th consecutive duplicate, got nil")
		}
		if !strings.Contains(err.Error(), "infinite loop") {
			t.Errorf("expected infinite loop error, got: %v", err)
		}
		if !isDup {
			t.Error("expected 4th consecutive duplicate to be detected")
		}
		if nag != "" {
			t.Errorf("expected empty nag on fatal duplicate, got %q", nag)
		}
		if rd.duplicateStreak != 0 {
			t.Errorf("expected duplicateStreak reset to 0 after skip, got %d", rd.duplicateStreak)
		}
	})

	// Scenario 2: Legitimate iteration — scanning different targets
	t.Run("legitimate iteration", func(t *testing.T) {
		rd := repetitionDetector{}
		targets := []string{
			`{"mode":"fast"}`,
			`{"mode":"deep","target":"192.168.50.10"}`,
			`{"mode":"deep","target":"192.168.50.1"}`,
			`{"mode":"deep","target":"192.168.50.60"}`,
			`{"mode":"deep","target":"192.168.50.63"}`,
			`{"mode":"deep","target":"192.168.50.125"}`,
			`{"mode":"deep","target":"192.168.50.241"}`,
		}
		for i, args := range targets {
			_, _, err := rd.check(logger, []proxy.ToolCall{makeCall("scan_local_network", args)})
			if err != nil {
				t.Fatalf("unexpected error at scan %d (%s): %v", i, args, err)
			}
		}
	})

	// Scenario 3: Consecutive same call is detected as duplicate
	t.Run("consecutive same call", func(t *testing.T) {
		rd := repetitionDetector{}

		_, _, err := rd.check(logger, []proxy.ToolCall{makeCall("execute_terminal_command",
			`{"command":"ts-node quick-check/test.ts","cwd":""}`)})
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}

		// Same command repeated consecutively — detected as duplicate
		isDup, nag, err := rd.check(logger, []proxy.ToolCall{makeCall("execute_terminal_command",
			`{"command":"ts-node quick-check/test.ts","cwd":""}`)})
		if err != nil {
			t.Fatalf("unexpected error on second call: %v", err)
		}
		if !isDup {
			t.Error("expected duplicate on consecutive identical call")
		}
		if nag == "" {
			t.Error("expected non-empty nag prompt")
		}
	})

	// Scenario 4: Different targets are NOT duplicates
	t.Run("different args not duplicate", func(t *testing.T) {
		rd := repetitionDetector{}

		_, _, err := rd.check(logger, []proxy.ToolCall{makeCall("scan_local_network",
			`{"mode":"fast"}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Different target — should NOT be duplicate
		isDup, _, err := rd.check(logger, []proxy.ToolCall{makeCall("scan_local_network",
			`{"mode":"deep","target":"192.168.50.10"}`)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isDup {
			t.Error("expected different scan targets not to be duplicates")
		}
	})

	// Scenario 5: submit_final_answer excluded from tracking
	t.Run("submit excluded", func(t *testing.T) {
		rd := repetitionDetector{}
		for range 6 {
			_, _, err := rd.check(logger, []proxy.ToolCall{{
				ID: "submit", Type: "function",
				Function: proxy.FunctionCall{Name: models.ToolSubmitFinalAnswer, Arguments: `{}`},
			}})
			if err != nil {
				t.Fatalf("submit_final_answer should never trigger loop: %v", err)
			}
		}
	})
}

func TestAgent_Execute_ToolExecutionErrorFeedback(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			hasToolResult := false
			var toolResultContent string
			for _, msg := range req.Messages {
				if msg.Role == proxy.ToolRole {
					hasToolResult = true
					toolResultContent = msg.Content
				}
			}

			if !hasToolResult {
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{{
						Message: proxy.Message{
							Role: "assistant",
							ToolCalls: []proxy.ToolCall{{
								ID:   "call_err",
								Type: "function",
								Function: proxy.FunctionCall{
									Name:      "test_tool",
									Arguments: `{"param": 1}`,
								},
							}},
						},
					}},
				}, nil
			}

			if !strings.Contains(toolResultContent, `"error"`) || !strings.Contains(toolResultContent, "simulated tool failure") {
				return nil, fmt.Errorf("expected tool error feedback in history, got: %s", toolResultContent)
			}

			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_submit",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "completed with failure handled"}`,
							},
						}},
					},
				}},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}

	engine := &MockEngine{
		Result: nil,
		Err:    fmt.Errorf("simulated tool failure"),
	}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})

	reply, history, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " run tool error test"},
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "completed with failure handled" {
		t.Errorf("expected reply 'completed with failure handled', got %q", reply)
	}

	foundToolError := false
	for _, msg := range history {
		if msg.Role == proxy.ToolRole && strings.Contains(msg.Content, "simulated tool failure") {
			foundToolError = true
		}
	}
	if !foundToolError {
		t.Error("expected tool error to be recorded in history")
	}
}

func TestToolCategory_AllCases(t *testing.T) {
	tests := []struct {
		toolName string
		expect   string
	}{
		{models.ToolTerminalExecute, "terminal"},
		{models.ToolDirectoryList, "filesystem"},
		{models.ToolFileRead, "filesystem"},
		{models.ToolFileWrite, "filesystem"},
		{models.ToolFileAppend, "filesystem"},
		{models.ToolNetworkFetch, "network"},
		{models.ToolNetworkScan, "network"},
		{models.ToolNetworkInfo, "network"},
		{models.ToolInternetSearch, "search"},
		{models.ToolNotifyUser, "communication"},
		{models.ToolApplyGuardrails, "general"},
		{models.ToolSubmitFinalAnswer, "general"},
		{models.ToolSystemError, "general"},
		{"unknown_tool", "general"},
		{"", "general"},
	}
	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			got := toolCategory(tc.toolName)
			if got != tc.expect {
				t.Errorf("toolCategory(%q) = %q, want %q", tc.toolName, got, tc.expect)
			}
		})
	}
}

func TestInjectToolInstructions_NoSystemMessage(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "hello"},
		{Role: proxy.AssistantRole, Content: "hi there"},
	}
	tools := []proxy.Tool{
		{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool", Description: "A test tool"}},
	}

	agent := &Agent{logger: logging.NewNopLogger()}
	result := agent.injectToolInstructions(history, tools)

	if len(result) != len(history)+1 {
		t.Errorf("expected %d messages, got %d", len(history)+1, len(result))
	}
	if result[0].Role != proxy.SystemRole {
		t.Errorf("expected first message to be system role, got %q", result[0].Role)
	}
	if !strings.Contains(result[0].Content, prompts.ToolManualHeader) {
		t.Errorf("expected system message to contain tool manual header, got %q", result[0].Content)
	}
}

func TestInjectToolInstructions_EmptyTools(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
		{Role: proxy.UserRole, Content: "hello"},
	}
	agent := &Agent{logger: logging.NewNopLogger()}
	result := agent.injectToolInstructions(history, nil)

	if len(result) != len(history) {
		t.Errorf("expected unchanged history length %d, got %d", len(history), len(result))
	}
	result = agent.injectToolInstructions(history, []proxy.Tool{})

	if len(result) != len(history) {
		t.Errorf("expected unchanged history length %d with empty tools, got %d", len(history), len(result))
	}
}

func TestNotifyPrematureTerminationNag(t *testing.T) {
	var events []AgentEvent
	agent := &Agent{
		observer: func(ev AgentEvent) { events = append(events, ev) },
	}
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "do something"},
	}
	agent.notifyPrematureTerminationNag(&history)

	if len(history) != 2 {
		t.Errorf("expected history length 2, got %d", len(history))
	}
	if history[1].Role != "user" {
		t.Errorf("expected nag message role 'user', got %q", history[1].Role)
	}
	if !strings.Contains(history[1].Content, "incomplete response") {
		t.Errorf("expected nag message to mention incomplete response, got %q", history[1].Content)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventMessage {
		t.Errorf("expected EventMessage event type, got %v", events[0].Type)
	}
}

func TestAgent_ExecutePlan_Success(t *testing.T) {
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role:    "assistant",
						Content: `{"description": "test plan", "steps": [{"tool": "test_tool", "description": "step 1", "args": {"key": "val"}}]}`,
					},
				}},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		},
	}
	engine := &MockEngine{Result: "step result"}

	strategy := NewExecutionPlanStrategy(client, provider.Tools, logging.NewNopLogger())
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		PlanStrategy:  strategy,
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "[Plan execution complete]" {
		t.Errorf("expected reply '[Plan execution complete]', got %q", reply)
	}
}

func TestComputeNextResponseStreamXML_PrefillThinkingError(t *testing.T) {
	// Test that when the stream returns a prefill thinking error,
	// it retries without prefill and succeeds.
	var streamCalls int
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCalls++
			if streamCalls == 1 {
				return nil, fmt.Errorf("prefill rejected: thinking mode")
			}
			ch := make(chan *proxy.ChatResponse, 2)
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
				Delta: proxy.Message{Content: "Hello world"},
			}}}
			close(ch)
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
		},
	}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		UsePrefill:    true,
		MaxSteps:      5,
		UseNativeTools: boolPtr(false),
	})

	msg, err := agent.computeNextResponseStreamXML(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
		{Role: proxy.AssistantRole, Content: "previous tool call"},
		{Role: proxy.ToolRole, Content: "result"},
	}, provider.Tools)
	if err != nil {
		t.Fatalf("computeNextResponseStreamXML failed: %v", err)
	}
	if msg.Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %q", msg.Content)
	}
	if streamCalls != 2 {
		t.Errorf("expected 2 stream calls (1 error + 1 success), got %d", streamCalls)
	}
}

func boolPtr(b bool) *bool { return &b }

func TestAgent_StuckThresholdConstant(t *testing.T) {
	tests := []struct {
		name        string
		maxRespTok  int
		reasoning   int // chars of reasoning to stream
		expectStuck bool // should stuck detection fire?
	}{
		{"below threshold with small max_tokens", 2730, 1500, false},
		{"below threshold with large max_tokens", 16384, 1500, false},
		{"above scaled threshold triggers detection", 2730, 6000, true},
		{"above floor threshold with small max_tokens", 512, 2500, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []AgentEvent
			streamCalls := 0
			chatCalls := 0
			client := &MockClient{
				StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
					streamCalls++
					ch := make(chan *proxy.ChatResponse, 100)
					go func() {
						defer close(ch)
						reasoningChunk := "the model thinks about the problem "
						charsSent := 0
						for charsSent < tc.reasoning {
							ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
								Delta: proxy.Message{ReasoningContent: reasoningChunk},
							}}}
							charsSent += len(reasoningChunk)
						}
					}()
					return ch, nil
				},
				ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
					chatCalls++
					return &proxy.ChatResponse{
						Choices: []proxy.Choice{{
							Message: proxy.Message{
								Role: "assistant",
								ToolCalls: []proxy.ToolCall{{
									ID:   "call_submit",
									Type: "function",
									Function: proxy.FunctionCall{
										Name:      models.ToolSubmitFinalAnswer,
										Arguments: `{"summary": "done"}`,
									},
								}},
							},
						}},
					}, nil
				},
			}
			provider := &MockProvider{
				Tools: []proxy.Tool{
					{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
				},
			}
			engine := &MockEngine{Result: "ok"}
			agent := NewAgent(client, provider, engine, AgentOptions{
				MaxSteps:          25,
				MaxResponseTokens: tc.maxRespTok,
			})
			agent.observer = func(ev AgentEvent) { events = append(events, ev) }
			_, _, err := agent.Execute(context.Background(), []proxy.Message{
				{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
			})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			// The fallback (ChatFunc) will be called regardless of threshold because
			// any stream that produces only reasoning with no content/tool calls
			// triggers the empty-stream fallback.  The threshold determines whether
			// stuck detection fires an explicit lifecycle event before the fallback.
			if chatCalls == 0 {
				t.Error("expected fallback (chat) to be called for reasoning-only stream")
			}
			foundStuckEvent := false
			for _, ev := range events {
				if ev.Type == EventLifecycle {
					if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == "stuck_detected" {
						foundStuckEvent = true
						break
					}
				}
			}
			if tc.expectStuck && !foundStuckEvent {
				t.Error("expected stuck_detected lifecycle event when reasoning exceeds threshold")
			}
			if !tc.expectStuck && foundStuckEvent {
				t.Error("unexpected stuck_detected lifecycle event when reasoning is below threshold")
			}
		})
	}
}

func TestAgent_StuckThresholdDerived(t *testing.T) {
	// The effective threshold is maxTokens * 2.
	// maxRespTok=8192 → threshold=16384.
	// reasoning=2000 should be BELOW that threshold (model not stuck),
	// while reasoning=20000 should be ABOVE (stuck detected).
	// Also test that the floor (2000) applies when tiny maxTokens.
	tests := []struct {
		name        string
		maxRespTok  int
		reasoning   int
		expectStuck bool
	}{
		{"below scaled threshold with large budget", 8192, 2000, false},
		{"above scaled threshold triggers detection", 8192, 20000, true},
		{"tiny budget still gets floor threshold", 256, 2500, true},
		{"tiny budget below floor does not trigger", 256, 1500, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []AgentEvent
			chatCalls := 0
			client := &MockClient{
				StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
					ch := make(chan *proxy.ChatResponse, 100)
					go func() {
						defer close(ch)
						reasoningChunk := "the model thinks about the problem "
						charsSent := 0
						for charsSent < tc.reasoning {
							ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
								Delta: proxy.Message{ReasoningContent: reasoningChunk},
							}}}
							charsSent += len(reasoningChunk)
						}
					}()
					return ch, nil
				},
				ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
					chatCalls++
					return &proxy.ChatResponse{
						Choices: []proxy.Choice{{
							Message: proxy.Message{
								Role: "assistant",
								ToolCalls: []proxy.ToolCall{{
									ID:   "call_submit",
									Type: "function",
									Function: proxy.FunctionCall{
										Name:      models.ToolSubmitFinalAnswer,
										Arguments: `{"summary": "done"}`,
									},
								}},
							},
						}},
					}, nil
				},
			}
			provider := &MockProvider{
				Tools: []proxy.Tool{
					{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
				},
			}
			engine := &MockEngine{Result: "ok"}
			agent := NewAgent(client, provider, engine, AgentOptions{
				MaxSteps:          25,
				MaxResponseTokens: tc.maxRespTok,
			})
			agent.observer = func(ev AgentEvent) { events = append(events, ev) }
			_, _, err := agent.Execute(context.Background(), []proxy.Message{
				{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
			})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if chatCalls == 0 {
				t.Error("expected fallback (chat) to be called for reasoning-only stream")
			}
			foundStuckEvent := false
			for _, ev := range events {
				if ev.Type == EventLifecycle {
					if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == "stuck_detected" {
						foundStuckEvent = true
						break
					}
				}
			}
			if tc.expectStuck && !foundStuckEvent {
				t.Error("expected stuck_detected lifecycle event when reasoning exceeds scaled threshold")
			}
			if !tc.expectStuck && foundStuckEvent {
				t.Error("unexpected stuck_detected lifecycle event when reasoning is below scaled threshold")
			}
		})
	}
}

func TestAgent_LifecycleEventsOnStuck(t *testing.T) {
	// When the reasoning stuck detector fires, a lifecycle event with
	// phase "stuck_detected" should be emitted.
	var events []AgentEvent
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 100)
			go func() {
				defer close(ch)
				for i := 0; i < 150; i++ {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
						Delta: proxy.Message{ReasoningContent: "model keeps thinking without producing anything "},
					}}}
				}
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_submit",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "done"}`,
							},
						}},
					},
				}},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	agent.observer = func(ev AgentEvent) { events = append(events, ev) }

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	foundStuck := false
	foundFallback := false
	for _, ev := range events {
		if ev.Type == EventLifecycle {
			payload, ok := ev.Payload.(map[string]any)
			if !ok {
				continue
			}
			if payload["phase"] == "stuck_detected" {
				foundStuck = true
				if _, ok := payload["reasoning_chars"]; !ok {
					t.Error("stuck_detected lifecycle event missing reasoning_chars")
				}
			}
			if payload["phase"] == "fallback_started" {
				foundFallback = true
			}
		}
	}
	if !foundStuck {
		t.Error("expected lifecycle event with phase stuck_detected")
	}
	if !foundFallback {
		t.Error("expected lifecycle event with phase fallback_started")
	}
}

func TestAgent_StuckDetectionExtractsToolCallFromReasoning(t *testing.T) {
	// When <tool_call> is embedded inside <think> content, the stuck
	// detector should extract it into Content so the XML parser can
	// process it, rather than declaring the model stuck.
	var events []AgentEvent
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 100)
			go func() {
				defer close(ch)
				// Reasoning content that far exceeds the threshold, but
				// contains an embedded <tool_call> block inside <think>.
				reasoning := `<think>The user wants to submit the final answer.
I should use the submit_final_answer tool to complete this.
<tool_call>{"tool": "` + models.ToolSubmitFinalAnswer + `", "args": {"summary": "done"}}</tool_call>
</think>
`
				for i := 0; i < 150; i++ {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
						Delta: proxy.Message{ReasoningContent: reasoning},
					}}}
				}
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			// Should NOT be called — the tool call is extracted from
			// reasoning, so no fallback needed.
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role:    "assistant",
						Content: "invalid fallback",
					},
				}},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          25,
		MaxResponseTokens: 2730,
	})
	agent.observer = func(ev AgentEvent) { events = append(events, ev) }
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	for _, ev := range events {
		if ev.Type == EventLifecycle {
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == "stuck_detected" {
				t.Error("unexpected stuck_detected event — tool call should have been extracted from reasoning")
			}
			if p, ok := ev.Payload.(map[string]any); ok && p["phase"] == "fallback_started" {
				t.Error("unexpected fallback_started event — extracted tool call should have been processed without fallback")
			}
		}
	}
}

func TestAgent_FallbackXMLModeNoToolChoice(t *testing.T) {
	// When the empty-stream-native-tools fallback fires, the non-streaming
	// retry should use XML mode: no tool_choice:required, no native tools.
	var capturedReq proxy.ChatRequest
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			capturedReq = req
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_submit",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolSubmitFinalAnswer,
								Arguments: `{"summary": "Task complete"}`,
							},
						}},
					},
				}},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolSubmitFinalAnswer}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// The fallback retry should have no native tools and no tool_choice
	if len(capturedReq.Tools) > 0 {
		t.Errorf("fallback should not use native tools in XML mode, got %d tools", len(capturedReq.Tools))
	}
	if capturedReq.ToolChoice != "" {
		t.Errorf("fallback should not set tool_choice in XML mode, got %q", capturedReq.ToolChoice)
	}
	if capturedReq.ReasoningBudget != 0 {
		t.Errorf("fallback should suppress ReasoningBudget in XML mode, got %d", capturedReq.ReasoningBudget)
	}
}

func TestTruncateLongContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		limit    int
		expected string
	}{
		{"short content unchanged", "hello", 100, "hello"},
		{"empty string unchanged", "", 100, ""},
		{"exact limit unchanged", "12345", 5, "12345"},
		{"zero limit unchanged", "hello", 0, "hello"},
		{"negative limit unchanged", "hello", -1, "hello"},
		{"long content truncated", "abcdefghijklmnopqrstuvwxyz", 10, "abcde\n...[Truncated]...\nvwxyz"},
		{"odd length limit", "abcdefghij", 5, "ab\n...[Truncated]...\nij"},
		{"one char each side", "abcdef", 2, "a\n...[Truncated]...\nf"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateLongContent(tc.input, tc.limit)
			if got != tc.expected {
				t.Errorf("truncateLongContent(%q, %d) = %q, want %q", tc.input, tc.limit, got, tc.expected)
			}
		})
	}
}

func TestAgent_SieveCompressionRange(t *testing.T) {
	// Compression targets messages between sieveLockedHead and len-sievePhysicalTail.
	// With 20 messages: compress indices [2:10] = 8 messages.
	// Middle messages have long content, head/tail have short content.
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
		{Role: proxy.UserRole, Content: "task"},
	}
	for i := 0; i < 8; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: fmt.Sprintf("middle %d: %s", i, strings.Repeat("x", 5000))})
	}
	for i := 0; i < 10; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: fmt.Sprintf("tail %d: short", i)})
	}

	agent := &Agent{contextBudget: 36000, logger: logging.NewNopLogger()}
	got := agent.applyPhysicalSieve(history)

	// Total before compression: ~40K chars → exceeds 36K budget.
	// After compression: middle 8 drop from 5K to 4K each → saves 8K → total ~32K → under budget, no drop.
	for _, m := range got {
		if strings.Contains(m.Content, prompts.SieveSystemNote) {
			t.Error("compression should have avoided dropping messages")
		}
	}
	if len(got) != len(history) {
		t.Errorf("expected all %d messages preserved after compression, got %d", len(history), len(got))
	}
	// Middle messages (indices 2-9) should have [Truncated] marker.
	for i := sieveLockedHead; i < len(history)-sievePhysicalTail; i++ {
		if !strings.Contains(got[i].Content, "...[Truncated]...") {
			t.Errorf("middle message [%d] should be truncated", i)
		}
	}
	// First 2 and last 10 should NOT be truncated.
	for i := 0; i < sieveLockedHead; i++ {
		if strings.Contains(got[i].Content, "...[Truncated]...") {
			t.Errorf("locked head message [%d] should not be truncated", i)
		}
	}
	for i := len(history) - sievePhysicalTail; i < len(history); i++ {
		if strings.Contains(got[i].Content, "...[Truncated]...") {
			t.Errorf("priority tail message [%d] should not be truncated", i)
		}
	}
}

func TestAgent_SieveCompressionAvoidsDrop(t *testing.T) {
	// Compression brings total just under budget → no sieve notes, all messages kept.
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
		{Role: proxy.UserRole, Content: "task"},
	}
	for i := 0; i < 8; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: fmt.Sprintf("msg %d: %s", i, strings.Repeat("x", 4100))})
	}
	for i := 0; i < 10; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: "short"})
	}

	agent := &Agent{contextBudget: 33000, logger: logging.NewNopLogger()}
	got := agent.applyPhysicalSieve(history)

	for _, m := range got {
		if strings.Contains(m.Content, prompts.ContextSieveWarning) {
			t.Error("sieve warning should not appear when compression avoids drop")
		}
	}
	if len(got) != len(history) {
		t.Errorf("expected %d messages, got %d", len(history), len(got))
	}
}

func TestAgent_SieveDropAfterCompressionExhausted(t *testing.T) {
	// Compression alone can't bring under budget → messages are dropped.
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
		{Role: proxy.UserRole, Content: "task"},
	}
	for i := 0; i < 24; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: strings.Repeat("x", 10000)})
	}

	agent := &Agent{contextBudget: 500, logger: logging.NewNopLogger()}
	got := agent.applyPhysicalSieve(history)

	foundWarning := false
	for _, m := range got {
		if strings.Contains(m.Content, prompts.ContextSieveWarning) {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected sieve warning when compression is insufficient")
	}
	if len(got) >= len(history) {
		t.Error("expected fewer messages after sieve drop")
	}
	expectedSize := sieveLockedHead + sievePhysicalTail + 2
	if len(got) != expectedSize {
		t.Errorf("expected %d messages after sieve, got %d", expectedSize, len(got))
	}
}

func TestAgent_SieveSafeEmptyRange(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"exactly locked head + tail", sieveLockedHead + sievePhysicalTail},
		{"below locked head + tail", 5},
		{"just one message", 1},
		{"empty history", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			history := make([]proxy.Message, tc.length)
			for i := range history {
				history[i] = proxy.Message{Role: proxy.UserRole, Content: strings.Repeat("x", 10000)}
			}
			agent := &Agent{contextBudget: 100, logger: logging.NewNopLogger()}
			got := agent.applyPhysicalSieve(history)
			if got == nil {
				t.Fatal("sieve should never return nil")
			}
		})
	}
}

func TestAgent_ReactiveSieveCount(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
		{Role: proxy.UserRole, Content: "task"},
	}
	for i := 0; i < 20; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: "msg"})
	}
	agent := &Agent{logger: logging.NewNopLogger()}
	got := agent.applyReactiveSieve(history)

	// sieveLockedHead + sieve note + sieveReactiveTail
	expected := sieveLockedHead + 1 + sieveReactiveTail
	if len(got) != expected {
		t.Errorf("expected %d messages after reactive sieve, got %d", expected, len(got))
	}
}

func TestAgent_AggressiveSieveCount(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system"},
		{Role: proxy.UserRole, Content: "task"},
	}
	for i := 0; i < 10; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: "msg"})
	}
	agent := &Agent{logger: logging.NewNopLogger()}
	got := agent.applyAggressiveSieve(history)

	// sieveLockedHead + sieve note + sieveAggressiveTail
	expected := sieveLockedHead + 1 + sieveAggressiveTail
	if len(got) != expected {
		t.Errorf("expected %d messages after aggressive sieve, got %d", expected, len(got))
	}
}

func TestAgent_SieveKeepsTaskAtLockedHead_AfterPlanStateInjection(t *testing.T) {
	// Simulates real automation history: [system, PlanState, user task, ...].
	// sieveLockedHead must be ≥3 to keep the task at [2] after PlanState injection at [1].
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "You are an autonomous agent..."},     // [0] system
		{Role: proxy.SystemRole, Content: "Goal: Execute task\n- [DONE] Step 1"}, // [1] PlanState
		{Role: proxy.UserRole, Content: "TASK: Run the smoke test steps..."},    // [2] task
	}
	for i := 0; i < 24; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: strings.Repeat("x", 10000)})
	}

	agent := &Agent{contextBudget: 500, logger: logging.NewNopLogger()}
	got := agent.applyPhysicalSieve(history)

	// Locked head contents must survive: system prompt, PlanState, and user task.
	if len(got) < 3 || got[0].Content != "You are an autonomous agent..." {
		t.Error("system prompt at [0] lost after sieve")
	}
	if len(got) < 3 || !strings.Contains(got[1].Content, "Goal: Execute task") {
		t.Error("PlanState at [1] lost after sieve")
	}
	if len(got) < 3 || !strings.Contains(got[2].Content, "TASK: Run the smoke test") {
		t.Error("user task at [2] lost after sieve — sieveLockedHead likely too small")
	}
}

func TestGuardrailDecisionStore_RegisterAndResolve(t *testing.T) {
	store := NewGuardrailDecisionStore()
	ch := make(chan GuardrailDecision, 1)
	store.Register("gr_1", ch)

	done := make(chan bool)
	go func() {
		ok := store.Resolve("gr_1", GuardrailDecision{Allow: true})
		if !ok {
			t.Error("Resolve returned false for registered decision")
		}
		done <- true
	}()

	select {
	case decision := <-ch:
		if !decision.Allow {
			t.Error("expected Allow=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for decision")
	}
	<-done
}

func TestGuardrailDecisionStore_ResolveNotFound(t *testing.T) {
	store := NewGuardrailDecisionStore()
	if store.Resolve("nonexistent", GuardrailDecision{Allow: true}) {
		t.Error("Resolve should return false for unknown ID")
	}
}

func TestGuardrailDecisionStore_ResolveTwiceFails(t *testing.T) {
	store := NewGuardrailDecisionStore()
	ch := make(chan GuardrailDecision, 2)
	store.Register("gr_2", ch)
	if !store.Resolve("gr_2", GuardrailDecision{Allow: true}) {
		t.Error("first Resolve should succeed")
	}
	if store.Resolve("gr_2", GuardrailDecision{Allow: false}) {
		t.Error("second Resolve should fail (already resolved)")
	}
}

func TestGuardrailDecisionStore_Remove(t *testing.T) {
	store := NewGuardrailDecisionStore()
	ch := make(chan GuardrailDecision, 1)
	store.Register("gr_3", ch)
	store.Remove("gr_3")
	if store.Resolve("gr_3", GuardrailDecision{Allow: true}) {
		t.Error("Resolve should fail after Remove")
	}
}

func TestGuardrailDecisionCallback_ContextCancelled(t *testing.T) {
	store := NewGuardrailDecisionStore()

	var mu sync.Mutex
	var events []AgentEvent
	observer := func(ev AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}

	callback := NewGuardrailDecisionCallback(store, observer)
	ctx, cancel := context.WithCancel(context.Background())

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_cancel_test",
		Tool:       "execute_terminal_command",
		Args:       `{"command":"sh test.sh"}`,
		Reason:     "command 'sh' not in whitelist",
		Category:   "terminal",
	}

	var decErr error
	done := make(chan struct{})
	go func() {
		_, decErr = callback(ctx, payload)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(events) == 0 {
		t.Fatal("expected EventGuardrailBlocked to be published")
	}
	blockedEvent := events[0]
	mu.Unlock()

	if blockedEvent.Type != EventGuardrailBlocked {
		t.Errorf("expected EventGuardrailBlocked, got %s", blockedEvent.Type)
	}

	blkPayload, ok := blockedEvent.Payload.(GuardrailBlockedPayload)
	if !ok {
		t.Fatal("blocked event payload has wrong type")
	}
	if blkPayload.DecisionID != "gr_cancel_test" {
		t.Errorf("expected DecisionID gr_cancel_test, got %s", blkPayload.DecisionID)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback did not return after context cancellation")
	}

	if decErr == nil {
		t.Error("expected error from cancelled context, got nil")
	}

	if store.Resolve("gr_cancel_test", GuardrailDecision{Allow: true}) {
		t.Error("decision should have been removed from store after cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range events {
		if ev.Type == EventGuardrailInvalidated {
			found = true
			invPayload, ok := ev.Payload.(GuardrailInvalidatedPayload)
			if !ok {
				t.Error("invalidation event payload has wrong type")
			}
			if invPayload.DecisionID != "gr_cancel_test" {
				t.Errorf("expected DecisionID gr_cancel_test, got %s", invPayload.DecisionID)
			}
			if invPayload.Reason != "context_cancelled" {
				t.Errorf("expected reason context_cancelled, got %s", invPayload.Reason)
			}
			break
		}
	}
	if !found {
		t.Error("expected EventGuardrailInvalidated to be published after cancellation")
	}
}

func TestGuardrailDecisionCallback_UserApprovesBeforeCancel(t *testing.T) {
	store := NewGuardrailDecisionStore()

	var events []AgentEvent
	var mu sync.Mutex
	observer := func(ev AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}

	callback := NewGuardrailDecisionCallback(store, observer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_approve_test",
		Tool:       "execute_terminal_command",
		Args:       `{"command":"ls"}`,
		Reason:     "command not in whitelist",
		Category:   "terminal",
	}

	var decision GuardrailDecision
	var cbErr error
	done := make(chan struct{})
	go func() {
		decision, cbErr = callback(ctx, payload)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	if !store.Resolve("gr_approve_test", GuardrailDecision{Allow: true, Persist: true}) {
		t.Fatal("Resolve should succeed for registered decision")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not return after user approval")
	}

	if cbErr != nil {
		t.Errorf("expected no error, got %v", cbErr)
	}
	if !decision.Allow {
		t.Error("expected Allow=true")
	}
	if !decision.Persist {
		t.Error("expected Persist=true")
	}
}

func TestGuardrailDecisionCallback_NoObserver(t *testing.T) {
	store := NewGuardrailDecisionStore()
	callback := NewGuardrailDecisionCallback(store, nil)
	ctx, cancel := context.WithCancel(context.Background())

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_no_observer",
		Tool:       "read_file",
		Args:       `{"path":"test.txt"}`,
		Reason:     "path not allowed",
		Category:   "filesystem",
	}

	done := make(chan struct{})
	go func() {
		callback(ctx, payload)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	store.Resolve("gr_no_observer", GuardrailDecision{Allow: false})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not return")
	}
	cancel()
}

func TestGuardrailDecisionCallback_WaitsIndefinitely(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping indefinite-wait test in short mode")
	}
	store := NewGuardrailDecisionStore()
	callback := NewGuardrailDecisionCallback(store, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_indefinite",
		Tool:       "execute_terminal_command",
		Args:       `{"command":"rm -rf /"}`,
		Reason:     "dangerous command",
		Category:   "terminal",
	}

	done := make(chan struct{})
	go func() {
		callback(ctx, payload)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("callback returned before user decision — it should block indefinitely")
	case <-time.After(200 * time.Millisecond):
	}

	store.Resolve("gr_indefinite", GuardrailDecision{Allow: false})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not return after resolve")
	}
}

func TestPrepareChatRequest_DynamicReasoningBudget(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "done"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	maxTokens := 4096
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          5,
		MaxResponseTokens: maxTokens,
		ReasoningBudget:   0, // explicitly zero — dynamic path should kick in
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " in workspace 'ws-1'.\nExecute the task."},
	}

	req := agent.buildChatRequest(prepared, nil, true)

	if agent.reasoningBudget != maxTokens/streamReasoningBudgetDivisor {
		t.Errorf("expected agent.reasoningBudget = %d, got %d",
			maxTokens/streamReasoningBudgetDivisor, agent.reasoningBudget)
	}
	if req.ReasoningBudget != maxTokens/streamReasoningBudgetDivisor {
		t.Errorf("expected req.ReasoningBudget = %d, got %d",
			maxTokens/streamReasoningBudgetDivisor, req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != maxTokens/streamReasoningBudgetDivisor {
		t.Errorf("expected req.ThinkingBudgetTokens = %d, got %d",
			maxTokens/streamReasoningBudgetDivisor, req.ThinkingBudgetTokens)
	}
}

func TestPrepareChatRequest_UsesExplicitReasoningBudget(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "done"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	explicitBudget := 1234
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          5,
		MaxResponseTokens: 4096,
		ReasoningBudget:   explicitBudget,
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " in workspace 'ws-1'.\nExecute the task."},
	}

	req := agent.buildChatRequest(prepared, nil, true)

	if agent.reasoningBudget != explicitBudget {
		t.Errorf("expected agent.reasoningBudget = %d, got %d", explicitBudget, agent.reasoningBudget)
	}
	if req.ReasoningBudget != explicitBudget {
		t.Errorf("expected req.ReasoningBudget = %d, got %d", explicitBudget, req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != explicitBudget {
		t.Errorf("expected req.ThinkingBudgetTokens = %d, got %d", explicitBudget, req.ThinkingBudgetTokens)
	}
}

func TestPrepareChatRequest_NonAutomation_SkipsReasoningBudget(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "done"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          5,
		MaxResponseTokens: 4096,
		ReasoningBudget:   0,
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: "plain chat message - not automation"},
	}

	req := agent.buildChatRequest(prepared, nil, false)

	if agent.reasoningBudget != 0 {
		t.Errorf("expected agent.reasoningBudget to remain 0 in non-automation, got %d", agent.reasoningBudget)
	}
	if req.ReasoningBudget != 0 {
		t.Errorf("expected req.ReasoningBudget to be 0 in non-automation, got %d", req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != 0 {
		t.Errorf("expected req.ThinkingBudgetTokens to be 0 in non-automation, got %d", req.ThinkingBudgetTokens)
	}
}

// boolPtr is defined earlier in this file (line ~2545)
