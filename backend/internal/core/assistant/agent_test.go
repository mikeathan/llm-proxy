package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
	"llm-proxy/internal/core/assistant/prompts"
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

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
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

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 10})
	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Write a large report"}})

	if err == nil {
		t.Fatal("expected error after exhausting retries with persistent tool call parse errors")
	}
	if !strings.Contains(err.Error(), "max steps") {
		t.Errorf("expected max steps exceeded error, got: %v", err)
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
					{Message: proxy.Message{Role: "assistant", Content: "Recovered after sieve."}},
				},
			}, nil
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

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
	if reply != "Recovered after sieve." {
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
		t.Fatal("expected max steps exceeded error")
	}
	if !strings.Contains(err.Error(), "max steps") {
		t.Errorf("expected max steps exceeded, got: %v", err)
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
		t.Fatal("expected max steps exceeded error in automation mode without tool calls")
	}
	if !strings.Contains(err.Error(), "max steps") {
		t.Errorf("expected max steps exceeded, got: %v", err)
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
