package assistant

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockClient implements proxy.Client
type MockClient struct {
	Response              proxy.ChatResponse
	Err                   error
	Calls                 int
	ChatFunc              func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error)
	StreamFunc            func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error)
	ReasoningFieldOverride string // empty => default (reasoning_budget)
}

func (m *MockClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, req)
	}
	// Fall back to non-streaming by default for existing tests
	return nil, fmt.Errorf("streaming not implemented in mock")
}

// ReasoningField reports the wire field this mock upstream expects. Defaults to
// the OpenAI-compatible "reasoning_budget"; tests for local llama.cpp set
// ReasoningFieldOverride to proxy.ReasoningFieldThinkTokens.
func (m *MockClient) ReasoningField() string {
	if m.ReasoningFieldOverride != "" {
		return m.ReasoningFieldOverride
	}
	return proxy.ReasoningFieldBudget
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
	Tools     []proxy.Tool
	UseNative *bool // nil = true (backward compat)
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
	Result   any
	Err      error
	Calls    int
	LastCall proxy.ToolCall
}

func (m *MockEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	m.Calls++
	m.LastCall = call
	return m.Result, m.Err
}

// captureLogger is a logging.Logger that captures all output into an in-memory
// buffer so tests can assert on log lines (e.g. absence of a spurious WARN).
type captureLogger struct {
	buf  *logging.BufferLogger
	base logging.Logger
}

func newCaptureLogger(limit int) *captureLogger {
	return &captureLogger{
		buf:  logging.NewBufferLogger(limit),
		base: logging.NewNopLogger(),
	}
}

func (c *captureLogger) Debug(msg string, args ...any) { c.buf.Debug(msg, args...); c.base.Debug(msg, args...) }
func (c *captureLogger) Info(msg string, args ...any)  { c.buf.Info(msg, args...); c.base.Info(msg, args...) }
func (c *captureLogger) Warn(msg string, args ...any)  { c.buf.Warn(msg, args...); c.base.Warn(msg, args...) }
func (c *captureLogger) Error(msg string, args ...any) { c.buf.Error(msg, args...); c.base.Error(msg, args...) }
func (c *captureLogger) With(args ...any) logging.Logger {
	cl := &captureLogger{buf: c.buf, base: c.base.With(args...)}
	return cl
}
func (c *captureLogger) SetLevel(l logging.Level) { c.base.SetLevel(l) }
func (c *captureLogger) Level() logging.Level     { return c.base.Level() }
func (c *captureLogger) String() string           { return c.buf.String() }

func TestAgent_Execute_Simple(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{
					Role:    "assistant",
					Content: "# Summary\nHello world",
				}},
			},
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})

	reply, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Hi"}})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "# Summary\nHello world" {
		t.Errorf("Expected '# Summary\nHello world', got '%s'", reply)
	}
	if len(history) != 2 { // user + assistant(content)
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
				{Message: proxy.Message{
					Role:    "assistant",
					Content: "# Summary\nIt is sunny in London.",
				}},
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
	if len(history) != 4 { // user + assistant(tc) + tool result + assistant(content)
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
			// First call: tool call. Second call: content-only (natural completion).
			if callCount == 1 {
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{{
						Message: proxy.Message{
							Role: "assistant",
							ToolCalls: []proxy.ToolCall{{
								ID:   "call_read",
								Type: "function",
								Function: proxy.FunctionCall{
									Name:      "read_file",
									Arguments: `{"path": "test.txt"}`,
								},
							}},
						},
					}},
				}, nil
			}
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role:    "assistant",
						Content: "Fallback response completed successfully",
					},
				}},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "file content"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 2 * time.Second,
	})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Hello"}})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "Fallback response completed successfully" {
		t.Errorf("expected 'Fallback response completed successfully', got '%s'", reply)
	}
	if callCount < 1 {
		t.Error("expected non-streaming fallback to be called")
	}
}

func TestAgent_Execute_ReasoningStuckFallback(t *testing.T) {
	// Stream returns only reasoning content (no text, no tool calls above threshold)
	// — should trigger the reactive sieve and retry.
	streamCount := 0
	var chatCallCount int
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
			chatCallCount++
			if chatCallCount == 1 {
				// After the sieve retries, produce a tool call
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "Task complete"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			// Second call: content-only signals completion
			msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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

func TestAgent_Execute_NativeToolsEmptyStreamFallsBackToNag(t *testing.T) {
	chatCalled := false
	streamCalls := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCalls++
			if streamCalls == 1 {
				// First call: empty stream
				ch := make(chan *proxy.ChatResponse)
				close(ch)
				return ch, nil
			}
			// Second+ calls (after nag and/or tool execution)
			ch := make(chan *proxy.ChatResponse, 3)
			go func() {
				defer close(ch)
				if streamCalls == 2 {
					// Return XML tool call (this is the XML retry from handleEmptyStream)
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
						Content: `<tool_call>
{"tool": "read_file", "args": {"summary": "Task complete"}}
</tool_call>`,
					}}}}
					return
				}
				// Third+ calls: content-only signals completion
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Task completed successfully"}}}}
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			chatCalled = true
			return nil, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 5 * time.Second,
		ProviderType:  "openai", // Prefill=false for this tier, prevents handleEmptyStream XML retry
	})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if chatCalled {
		t.Error("no non-streaming fallback should be called for native-only models")
	}
	if !agent.config.UseNativeTools {
		t.Error("agent should restore native tools after empty-stream fallback")
	}
}

// TestAgent_ComputeNextResponse_EmptyFinalizationSkipsNonStream verifies that
// when the recovery ladder has already forced a text-only finalization turn
// (finalizeAttempts >= 1) and that turn comes back empty, handleEmptyStream
// must NOT issue a redundant non-stream request — it returns the stuck signal
// so handleNoToolCalls terminates via bestAvailableAnswer instead. Without the
// gate, the wasted non-stream call can hit an upstream 5xx and become the
// run's terminal error, masking the graceful ladder termination.
func TestAgent_ComputeNextResponse_EmptyFinalizationSkipsNonStream(t *testing.T) {
	chatCalled := false
	streamCalls := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCalls++
			// Empty stream — no content, no reasoning, no tool calls.
			ch := make(chan *proxy.ChatResponse)
			close(ch)
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			chatCalled = true
			return nil, fmt.Errorf("non-stream fallback must not be called after finalization")
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}}},
	}
	agent := NewAgent(client, provider, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 5 * time.Second,
		ProviderType:  "openai", // Prefill=false for this tier — prefill would populate Content and skip handleEmptyStream.
	})
	// Reproduce the state AFTER the ladder armed and fired the text-only
	// finalization turn: finalizeAttempts already 1, this call IS the
	// finalization turn (tools disabled).
	agent.runS = &runSession{finalizeAttempts: 1}

	msg, err := agent.computeNextResponse(context.Background(),
		[]proxy.Message{{Role: proxy.UserRole, Content: "summarize the files"}},
		nil, // tools disabled — text-only finalization turn
		proxy.ToolChoiceNone,
	)
	if err != nil {
		t.Fatalf("expected graceful stuck signal, got error: %v", err)
	}
	if streamCalls != 1 {
		t.Errorf("expected exactly 1 stream attempt, got %d", streamCalls)
	}
	if chatCalled {
		t.Error("non-stream fallback must not be called when finalizeAttempts >= 1")
	}
	if msg.ReasoningContent != "[stuck]" {
		t.Errorf("expected ReasoningContent '[stuck]', got %q", msg.ReasoningContent)
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
		{
			"assistant after tool with recovery nag between",
			[]proxy.Message{
				{Role: proxy.ToolRole, Content: `"result data"`},
				{Role: proxy.UserRole, Content: prompts.AutomationNagPrompt},
				{Role: proxy.AssistantRole, Content: "Got the result, here's what I found."},
			},
			true,
		},
		{
			"assistant after tool with stuck placeholder and nag",
			[]proxy.Message{
				{Role: proxy.ToolRole, Content: `"result data"`},
				{Role: proxy.AssistantRole, Content: "", ReasoningContent: "[stuck]"},
				{Role: proxy.UserRole, Content: prompts.AutomationNagPrompt},
				{Role: proxy.AssistantRole, Content: "Final answer after recovery path."},
			},
			true,
		},
		{
			"real user after tool is not skipped",
			[]proxy.Message{
				{Role: proxy.ToolRole, Content: `"result data"`},
				{Role: proxy.UserRole, Content: "Please also check the second file."},
				{Role: proxy.AssistantRole, Content: "I will check that next after this note."},
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
			if callCount == 2 {
				tc := proxy.ToolCall{Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "Report written successfully"}`}}
				msg := proxy.Message{Role: "assistant", Content: "Here is the final result.", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Report written successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls (1 error + 1 retry + 1 completion), got %d", callCount)
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

// TestAgent_ToolCallParseError_EmptyXMLBlock_Recovers verifies that when the
// agent receives a model response with an empty <tool_call></tool_call> block
// (the model planned to act but the call was malformed), the agent does NOT
// treat the accompanying plan text as a natural completion.  It recovers by
// feeding back corrective guidance, then continues with valid tool calls and
// eventually the correct final answer.
func TestAgent_ToolCallParseError_EmptyXMLBlock_Recovers(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// Turn 1: model produces plan text with an EMPTY <tool_call> block.
				// This mimics the exact failure from gen/workspace-test/Qwen3.5 runs.
				return &proxy.ChatResponse{Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role:    "assistant",
						Content: "TypeScript is available. Now I'll compile.\n\n<tool_call>\n\n</tool_call>",
					},
				}}}, nil
			}
			if callCount == 2 {
				// Turn 2: model recovers with a valid native tool call.
				return &proxy.ChatResponse{Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: proxy.FunctionCall{
									Name:      "read_file",
									Arguments: `{"path": "test.ts"}`,
								},
							},
						},
					},
				}}}, nil
			}
			// Turn 3: final answer — plain text, no tool calls.
			return &proxy.ChatResponse{Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role:    "assistant",
					Content: "## Build successful\n\nAll tests pass.",
				},
			}}}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "file read successful"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 5 * time.Second,
	})

	// Seed history with a tool result so that precededByToolResult would be
	// satisfied — mirroring the real-world scenario where the model just
	// received a tool result and is about to take the next action.
	initialHistory := []proxy.Message{
		{Role: "user", Content: "Write and compile a TypeScript app"},
		{Role: "assistant", ToolCalls: []proxy.ToolCall{
			{ID: "init", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{}`}}}},
		{Role: "tool", Content: `{"stdout": "ok"}`},
	}

	reply, _, err := agent.Execute(context.Background(), initialHistory)

	if err != nil {
		t.Fatalf("Execute should have recovered from tool call parse error, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls (1 parse error + 1 retry + 1 completion), got %d", callCount)
	}
	if !strings.Contains(reply, "Build successful") {
		t.Errorf("final answer should contain 'Build successful', got '%s'", reply)
	}
	if strings.Contains(reply, "TypeScript is available") {
		t.Errorf("final answer must NOT contain plan text from the broken tool-call turn, got '%s'", reply)
	}
}

// TestAgent_ToolCallParseError_TruncatedXMLBlock verifies that when the model
// produces a truncated/incomplete tool call (missing closing tag) after a tool
// result, the agent does NOT treat the preceding plan text as a natural
// completion.  Neither regex parser matches the incomplete tag, so the
// hasToolCallMarker fallback in handleContentToolCalls must detect the intent
// and route to parse-error recovery instead.
func TestAgent_ToolCallParseError_TruncatedXMLBlock(t *testing.T) {
	callCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// Turn 1: model produces plan text with a TRUNCATED tool call
				// (no closing </tool_call>).  Neither regex matches, so
				// XMLFound=false — hasToolCallMarker catches the intent.
				return &proxy.ChatResponse{Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role:    "assistant",
						Content: "The temperature is 22°C. Let me check more.\n<tool_call>{\"tool\":\"get_more\"",
					},
				}}}, nil
			}
			if callCount == 2 {
				return &proxy.ChatResponse{Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role: "assistant",
						ToolCalls: []proxy.ToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: proxy.FunctionCall{
									Name:      "read_file",
									Arguments: `{"path": "data.ts"}`,
								},
							},
						},
					},
				}}}, nil
			}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role:    "assistant",
					Content: "## Results\n\nAll data checked.",
				},
			}}}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 5 * time.Second,
	})

	initialHistory := []proxy.Message{
		{Role: "user", Content: "Check all data files"},
		{Role: "assistant", ToolCalls: []proxy.ToolCall{
			{ID: "init", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{}`}}}},
		{Role: "tool", Content: `{"stdout": "ok"}`},
	}

	reply, _, err := agent.Execute(context.Background(), initialHistory)

	if err != nil {
		t.Fatalf("Execute should have recovered from truncated tool call, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls (1 truncated + 1 retry + 1 completion), got %d", callCount)
	}
	if !strings.Contains(reply, "Results") {
		t.Errorf("final answer should contain 'Results', got '%s'", reply)
	}
	if strings.Contains(reply, "Let me check more") {
		t.Errorf("final answer must NOT contain plan text from truncated tool-call turn, got '%s'", reply)
	}
}

// TestAgent_Execute_PlainTextFinalAnswer_NoParseErrorWARN verifies that a
// native-tools model answering directly with a plain-text final report does
// NOT log the spurious "tool call parse error" WARN that previously fired on
// every final report (seen on laguna and nemotron runs). The parseErr from
// handleContentToolCalls has XMLFound=false for plain text, which is a normal
// completion — not a parse error.
func TestAgent_Execute_PlainTextFinalAnswer_NoParseErrorWARN(t *testing.T) {
	logBuf := newCaptureLogger(64 * 1024)
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					Content: "Here is the full report of all workspace files:\n\n- network-recon-report.md\n- ts-dashboard/app.ts\n- ts-dashboard/report.md",
				}}}}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	agent := NewAgent(client, provider, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 5 * time.Second,
		Logger:        logBuf,
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "list all files and report"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(reply, "full report") {
		t.Errorf("expected final report in reply, got %q", reply)
	}
	if strings.Contains(logBuf.String(), "tool call parse error") {
		t.Errorf("plain-text final answer must not log 'tool call parse error', got:\n%s", logBuf.String())
	}
}

// TestAgent_ToolStreamGrowsPastCodeFence verifies that the live streamed
// content (EventToolStream) keeps growing past a markdown code fence. A bare
// ``` fence is legitimate report formatting and must NOT be treated as a
// tool-call cutoff — previously the visible stream froze at the first fence
// while the model kept generating, making the UI appear stuck.
func TestAgent_ToolStreamGrowsPastCodeFence(t *testing.T) {
	const fence = "Here is the report:\n```\ncode block\n```\nAnd the rest of the content after the fence."
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 3)
			go func() {
				defer close(ch)
				// Stream in two chunks: first half ends mid-fence, second half completes it.
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					Content: "Here is the report:\n```\ncode b",
				}}}}
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{
					Content: "lock\n```\nAnd the rest of the content after the fence.",
				}}}}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}}},
	}

	var streamPayloads []string
	agent := NewAgent(client, provider, &MockEngine{Result: "ok"}, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 5 * time.Second,
		Observer: func(ev AgentEvent) {
			if ev.Type == EventToolStream {
				if s, ok := ev.Payload.(string); ok {
					streamPayloads = append(streamPayloads, s)
				}
			}
		},
	})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "list all files and report"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(streamPayloads) < 2 {
		t.Fatalf("expected multiple tool_stream payloads, got %d", len(streamPayloads))
	}
	last := streamPayloads[len(streamPayloads)-1]
	if last != fence {
		t.Errorf("tool_stream content must grow past the code fence, want %q, got %q", fence, last)
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

func TestIsUnsupportedParameterError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"arbitrary", fmt.Errorf("connection refused"), false},
		{"nvidia real error", fmt.Errorf(`LLM chat error 400: {"error":{"message":"Validation: Unsupported parameter(s): ` + "`thinking_budget_tokens`" + `","type":"Bad Request","code":400}}`), true},
		{"simple unsupported", fmt.Errorf("Unsupported parameter: thinking_budget_tokens"), true},
		{"tool support error not matched", fmt.Errorf("tools is not currently supported"), false},
		{"context size error not matched", fmt.Errorf("context size exceeded"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUnsupportedParameterError(tt.err)
			if got != tt.want {
				t.Errorf("isUnsupportedParameterError() = %v, want %v", got, tt.want)
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
			{Role: proxy.AssistantRole, Content: "Task completed successfully"},
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
			if callCount == 2 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "Recovered after sieve."}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Recovered after sieve."}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls (1 error + 1 retry + 1 completion), got %d", callCount)
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
			if callCount == 2 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "Fallback without tools."}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Fallback without tools."}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Hi"}})

	if err != nil {
		t.Fatalf("Execute should have recovered from tool support error, got: %v", err)
	}
	if reply != "Fallback without tools." {
		t.Errorf("expected fallback response, got '%s'", reply)
	}
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls (1 error + 1 retry + 1 completion), got %d", callCount)
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
			if callCount == 1 {
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "I do not know about tools."}}}}, nil
			}
			if callCount == 2 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "done"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 12})

	// Track notifications by capturing the observer
	var events []AgentEvent
	agent.deps.Observer = func(ev AgentEvent) {
		events = append(events, ev)
	}

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " read the file"},
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
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

	history := []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " some task"},
	}

	reply, _, err := agent.Execute(context.Background(), history)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "Chatting instead of using tools." {
		t.Errorf("expected model output, got: %s", reply)
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
					{Message: proxy.Message{
						Role:    "assistant",
						Content: "The weather in London is sunny and warm today.",
					}},
				},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "get_weather"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: `"Sunny"`}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	reply, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "Weather?"}})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "The weather in London is sunny and warm today." {
		t.Errorf("expected final response, got '%s'", reply)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}
}

func TestNotifyPrefillDisabled(t *testing.T) {
	agent := &Agent{}
	var events []AgentEvent
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

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
			agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

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
				{Message: proxy.Message{
					Role:    "assistant",
					Content: "# Summary\nDone",
				}},
			},
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{},
	}
	engine := &MockEngine{Result: "ok"}

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

func TestCheckTaskCompletion(t *testing.T) {
	// Natural completion (Hermes-aligned): the model emits a content-only
	// assistant message with no tool calls.  Completion requires substantive
	// visible text (≥20 chars after think-block stripping) and at least one
	// tool result anywhere in history — not strict adjacency to the prior message.
	agent := NewAgent(&MockClient{StreamFunc: func(context.Context, proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
		return nil, context.Canceled
	}}, &MockProvider{}, &MockEngine{}, AgentOptions{})
	_ = newRunSession(agent, context.Background(), nil)

	toolResult := proxy.Message{Role: proxy.ToolRole, Content: "file contents"}
	assistantText := proxy.Message{Role: proxy.AssistantRole, Content: "planning next step"}

	tests := []struct {
		name        string
		msg         proxy.Message
		history     []proxy.Message
		wantDone    bool
		wantContent string
	}{
		{
			name:        "content + no toolcalls + tool result in history => done",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "# Report\nTask completed successfully.", ToolCalls: nil},
			history:     []proxy.Message{toolResult},
			wantDone:    true,
			wantContent: "# Report\nTask completed successfully.",
		},
		{
			name:        "content + toolcalls => not done",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "# Report\nTask completed successfully.", ToolCalls: []proxy.ToolCall{{Function: proxy.FunctionCall{Name: "read_file"}}}},
			history:     []proxy.Message{toolResult},
			wantDone:    false,
			wantContent: "",
		},
		{
			name:        "empty content => not done",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "   ", ToolCalls: nil},
			history:     []proxy.Message{toolResult},
			wantDone:    false,
			wantContent: "",
		},
		{
			name:        "think-only content => not done (stripped to empty)",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "<think>reasoning here</think>", ToolCalls: nil},
			history:     []proxy.Message{toolResult},
			wantDone:    false,
			wantContent: "",
		},
		{
			name:        "think-block + visible content => done (stripped to visible)",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "<think>reasoning</think># Final Report\nWork completed.", ToolCalls: nil},
			history:     []proxy.Message{toolResult},
			wantDone:    true,
			wantContent: "# Final Report\nWork completed.",
		},
		{
			name:        "no tool result in history => not done (premature guard)",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "# Report\nWork completed successfully.", ToolCalls: nil},
			history:     []proxy.Message{assistantText},
			wantDone:    false,
			wantContent: "",
		},
		{
			name:        "reasoning-only interleave does not block completion",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "# Final Report\n\nAll tasks completed successfully.", ToolCalls: nil},
			history:     []proxy.Message{toolResult, {Role: proxy.AssistantRole, Content: "", ReasoningContent: "reasoning..."}},
			wantDone:    true,
			wantContent: "# Final Report\n\nAll tasks completed successfully.",
		},
		{
			name:        "unparsed tool_call marker in content => not done",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "<tool_call>{\"tool\":\"read_file\"}</tool_call> maybe also some text here so it's long enough", ToolCalls: nil},
			history:     []proxy.Message{toolResult},
			wantDone:    false,
			wantContent: "",
		},
		{
			name:        "short chatter <20 chars => not done",
			msg:         proxy.Message{Role: proxy.AssistantRole, Content: "Let me check.", ToolCalls: nil},
			history:     []proxy.Message{toolResult},
			wantDone:    false,
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, done := checkTaskCompletion(tt.msg, tt.history)
			if done != tt.wantDone {
				t.Errorf("expected done=%v, got %v", tt.wantDone, done)
			}
			if got != tt.wantContent {
				t.Errorf("expected content=%q, got %q", tt.wantContent, got)
			}
		})
	}
}

func TestIsAgentControlMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  proxy.Message
		want bool
	}{
		{"automation nag", proxy.Message{Role: proxy.UserRole, Content: prompts.AutomationNagPrompt}, true},
		{"sieve warning", proxy.Message{Role: proxy.UserRole, Content: prompts.ContextSieveWarning}, true},
		{"sieve note", proxy.Message{Role: proxy.UserRole, Content: prompts.SieveSystemNote}, true},
		{"stuck nag", proxy.Message{Role: proxy.UserRole, Content: prompts.ReasoningStuckNag}, true},
		{"retry prefix", proxy.Message{Role: proxy.UserRole, Content: prompts.RetrySignal + "\n\noriginal user"}, true},
		{"parse feedback STOP", proxy.Message{Role: proxy.UserRole, Content: "STOP writing text. Produce a tool call NOW."}, true},
		{"parse feedback FORMAT", proxy.Message{Role: proxy.UserRole, Content: "FORMAT ERROR: bad json"}, true},
		{"stuck placeholder", proxy.Message{Role: proxy.AssistantRole, Content: "", ReasoningContent: "[stuck]"}, true},
		{"real user", proxy.Message{Role: proxy.UserRole, Content: "Please scan the network and report."}, false},
		{"user mentioning SYSTEM ERROR casually", proxy.Message{Role: proxy.UserRole, Content: "SYSTEM ERROR: foo happened in my app"}, false},
		{"assistant content", proxy.Message{Role: proxy.AssistantRole, Content: "Here is the final report body."}, false},
		{"tool result", proxy.Message{Role: proxy.ToolRole, Content: "scan complete"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAgentControlMessage(tt.msg); got != tt.want {
				t.Errorf("isAgentControlMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPreviousConversationMessage(t *testing.T) {
	tool := proxy.Message{Role: proxy.ToolRole, Content: "result"}
	userTask := proxy.Message{Role: proxy.UserRole, Content: "do the work"}
	nag := proxy.Message{Role: proxy.UserRole, Content: prompts.AutomationNagPrompt}
	stuck := proxy.Message{Role: proxy.AssistantRole, Content: "", ReasoningContent: "[stuck]"}
	sieve := proxy.Message{Role: proxy.UserRole, Content: prompts.ContextSieveWarning}
	assistant := proxy.Message{Role: proxy.AssistantRole, Content: "planning next step"}

	t.Run("skips nag to tool", func(t *testing.T) {
		prev := previousConversationMessage([]proxy.Message{tool, nag})
		if prev == nil || prev.Role != proxy.ToolRole {
			t.Fatalf("expected tool, got %#v", prev)
		}
	})
	t.Run("skips stuck and nag and sieve to tool", func(t *testing.T) {
		prev := previousConversationMessage([]proxy.Message{tool, stuck, nag, sieve})
		if prev == nil || prev.Role != proxy.ToolRole {
			t.Fatalf("expected tool, got %#v", prev)
		}
	})
	t.Run("returns user task under nag", func(t *testing.T) {
		prev := previousConversationMessage([]proxy.Message{userTask, nag})
		if prev == nil || prev.Content != userTask.Content {
			t.Fatalf("expected user task, got %#v", prev)
		}
	})
	t.Run("does not skip real assistant", func(t *testing.T) {
		prev := previousConversationMessage([]proxy.Message{tool, assistant})
		if prev == nil || prev.Role != proxy.AssistantRole {
			t.Fatalf("expected assistant, got %#v", prev)
		}
	})
	t.Run("only control returns nil", func(t *testing.T) {
		prev := previousConversationMessage([]proxy.Message{nag, sieve, stuck})
		if prev != nil {
			t.Fatalf("expected nil, got %#v", prev)
		}
	})
	t.Run("semantic prev enables completion after recovery", func(t *testing.T) {
		history := []proxy.Message{tool, stuck, nag}
		msg := proxy.Message{Role: proxy.AssistantRole, Content: "# Report\nTask completed successfully."}
		// checkTaskCompletion now takes the history directly (Phase 2b: Hermes-aligned).
		// The tool result in history[0] satisfies the any-tool-result gate.
		got, done := checkTaskCompletion(msg, history)
		if !done || got != msg.Content {
			t.Fatalf("expected completion, done=%v content=%q", done, got)
		}
	})
}

func TestAgent_Execute_CompletesThroughRecoveryPollution(t *testing.T) {
	// Reproduces: tool result → stuck placeholder → nag → final text must complete
	// (not re-nag and re-emit the answer forever).
	var callCount int
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			return &proxy.ChatResponse{Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role:    "assistant",
					Content: "The network reconnaissance task has been completed successfully.",
				},
			}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	agent := NewAgent(client, provider, &MockEngine{}, AgentOptions{MaxSteps: 5})

	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "Run recon and deliver the report."},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{
			{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path":"task.md"}`}},
		}},
		{Role: proxy.ToolRole, Content: "scan data ready"},
		{Role: proxy.AssistantRole, Content: "", ReasoningContent: "[stuck]"},
		{Role: proxy.UserRole, Content: prompts.AutomationNagPrompt},
	}

	reply, _, err := agent.Execute(context.Background(), history)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected single LLM call after recovery pollution, got %d", callCount)
	}
	if !strings.Contains(reply, "completed successfully") {
		t.Errorf("expected final answer reply, got %q", reply)
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "success"}

	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			Global: models.GlobalGuardrailsConfig{
				BlockSecrets: true,
			},
		}
	}, storage.NewPathResolver("", "", ""), nil, nil)

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
						Role:    "assistant",
						Content: "Succeeded",
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "success"}

	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			Global: models.GlobalGuardrailsConfig{
				BlockSecrets: true,
			},
		}
	}, storage.NewPathResolver("", "", ""), nil, nil)

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
						Role:    "assistant",
						Content: "Denied",
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
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "File read.\nTask complete"}}}}
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
								Name:      "read_file",
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
	streamCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			capturedReq = req
			streamCount++
			ch := make(chan *proxy.ChatResponse, 2)
			go func() {
				defer close(ch)
				if streamCount == 1 {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "read_file", "args": {"path": "test.txt"}}
</tool_call>`}}}}
				} else {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Done"}}}}
				}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
	if capturedReq.ToolChoice != "" {
		t.Errorf("expected empty ToolChoice (tool_choice:required removed — natural completion), got %q", capturedReq.ToolChoice)
	}
	if capturedReq.Temperature != 0 {
		t.Errorf("expected Temperature=0 (not configured), got %f", capturedReq.Temperature)
	}
	if capturedReq.ReasoningBudget != 0 {
		t.Errorf("expected zero ReasoningBudget (not configured), got %d", capturedReq.ReasoningBudget)
	}
}

func TestAgent_Execute_NativeToolsTemperatureSet(t *testing.T) {
	var capturedReq proxy.ChatRequest
	streamCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			capturedReq = req
			streamCount++
			ch := make(chan *proxy.ChatResponse, 2)
			go func() {
				defer close(ch)
				if streamCount == 1 {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "read_file", "args": {"path": "test.txt"}}
</tool_call>`}}}}
				} else {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Done"}}}}
				}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
	if capturedReq.Temperature != 0 {
		t.Errorf("expected Temperature=0 (not configured), got %f", capturedReq.Temperature)
	}
}

func TestAgent_Execute_XMLToolChoiceUnset(t *testing.T) {
	var capturedReq proxy.ChatRequest
	streamCount := 0
	falseVal := false
	client := &MockClient{
		// Fall back to non-streaming so prefill doesn't corrupt the capture
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			capturedReq = req
			streamCount++
			ch := make(chan *proxy.ChatResponse, 2)
			go func() {
				defer close(ch)
				if streamCount == 1 {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: `<tool_call>
{"tool": "read_file", "args": {"path": "test.txt"}}
</tool_call>`}}}}
				} else {
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Done"}}}}
				}
			}()
			return ch, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
	if capturedReq.ToolChoice == "required" {
		t.Errorf("XML mode must not force ToolChoice=required, got %q", capturedReq.ToolChoice)
	}
	if capturedReq.Temperature != 0 {
		t.Errorf("expected Temperature=0 (not configured), got %f", capturedReq.Temperature)
	}
	if capturedReq.ReasoningBudget != 0 {
		t.Errorf("expected zero ReasoningBudget (not configured), got %d", capturedReq.ReasoningBudget)
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

	// Scenario 5: system_error excluded from tracking (no-op bookkeeping tool)
	t.Run("system_error excluded", func(t *testing.T) {
		rd := repetitionDetector{}
		for range 6 {
			_, _, err := rd.check(logger, []proxy.ToolCall{{
				ID: "sys", Type: "function",
				Function: proxy.FunctionCall{Name: models.ToolSystemError, Arguments: `{}`},
			}})
			if err != nil {
				t.Fatalf("system_error should never trigger loop: %v", err)
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
						Role:    "assistant",
						Content: "completed with failure handled",
					},
				}},
			}, nil
		},
	}

	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "test_tool"}},
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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
		{"read_file", "filesystem"},
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

	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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
		deps: AgentRuntimeDeps{
			Observer: func(ev AgentEvent) { events = append(events, ev) },
		},
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
		MaxSteps:     5,
		PlanStrategy: strategy,
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
		UsePrefill:     true,
		MaxSteps:       5,
		UseNativeTools: boolPtr(false),
	})

	msg, err := agent.computeNextResponseStreamXML(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
		{Role: proxy.AssistantRole, Content: "previous tool call"},
		{Role: proxy.ToolRole, Content: "result"},
	}, provider.Tools, "")
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
		reasoning   int  // chars of reasoning to stream
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
					if chatCalls == 1 {
						tc := proxy.ToolCall{ID: "call_read", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path": "test.txt"}`}}
						msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
						return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
					}
					msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
					return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
				},
			}
			provider := &MockProvider{
				Tools: []proxy.Tool{
					{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
				},
			}
			engine := &MockEngine{Result: "ok"}
			agent := NewAgent(client, provider, engine, AgentOptions{
				MaxSteps:          25,
				MaxResponseTokens: tc.maxRespTok,
				ReasoningBudget:   tc.maxRespTok * 2,
			})
			agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }
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
					if chatCalls == 1 {
						tc := proxy.ToolCall{ID: "call_read", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path": "test.txt"}`}}
						msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
						return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
					}
					msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
					return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
				},
			}
			provider := &MockProvider{
				Tools: []proxy.Tool{
					{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
				},
			}
			engine := &MockEngine{Result: "ok"}
			agent := NewAgent(client, provider, engine, AgentOptions{
				MaxSteps:          25,
				MaxResponseTokens: tc.maxRespTok,
				ReasoningBudget:   tc.maxRespTok * 2,
			})
			agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }
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
	var chatCallCount int
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
			chatCallCount++
			if chatCallCount == 1 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "done"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{MaxSteps: 5})
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

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
	var streamCallCount int
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCallCount++
			ch := make(chan *proxy.ChatResponse, 100)
			go func() {
				defer close(ch)
				if streamCallCount == 1 {
					// Reasoning content that far exceeds the threshold, but
					// contains an embedded <tool_call> block inside <think>.
					reasoning := `<think>The user wants to submit the final answer.
I should use the submit_final_answer tool to complete this.
<tool_call>{"tool": "` + "read_file" + `", "args": {"summary": "done"}}</tool_call>
</think>
`
					for i := 0; i < 150; i++ {
						ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
							Delta: proxy.Message{ReasoningContent: reasoning},
						}}}
					}
					return
				}
				// Second stream call (after tool extraction + execution):
				// content-only signals completion
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Task completed successfully"}}}}
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
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          25,
		MaxResponseTokens: 2730,
	})
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }
	_, finalHistory, err := agent.Execute(context.Background(), []proxy.Message{
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

	// Verify reasoning text was NOT promoted to visible Content in history.
	// The llama.cpp server already separates reasoning_content from content;
	// reasoning text belongs to thinking, not visible assistant output.
	for i, msg := range finalHistory {
		if msg.Role == proxy.AssistantRole && strings.Contains(msg.Content, "submit") {
			t.Errorf("history[%d] contains extracted reasoning text: %q", i, msg.Content)
		}
	}
}

func TestCheckStreamStuck_NonReasoningModel(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			MaxTokens:       2730,
			ReasoningBudget: 0, // non-reasoning model
			SkipStuckCheck:  false,
		},
	}

	t.Run("under threshold not stuck", func(t *testing.T) {
		msg := &proxy.Message{ReasoningContent: strings.Repeat("x", 1000)}
		if agent.checkStreamStuck(msg) {
			t.Error("1000 chars should not trigger early stuck")
		}
	})

	t.Run("above threshold stuck", func(t *testing.T) {
		msg := &proxy.Message{ReasoningContent: strings.Repeat("x", 2731)}
		if !agent.checkStreamStuck(msg) {
			t.Error("2731 chars should trigger early stuck for non-reasoning model (threshold = maxTokens = 2730)")
		}
	})

	t.Run("has content not stuck", func(t *testing.T) {
		msg := &proxy.Message{Content: "text", ReasoningContent: strings.Repeat("x", 2731)}
		if agent.checkStreamStuck(msg) {
			t.Error("has content -> not stuck even above early threshold")
		}
	})

	t.Run("has tool calls not stuck", func(t *testing.T) {
		msg := &proxy.Message{ToolCalls: []proxy.ToolCall{{}}, ReasoningContent: strings.Repeat("x", 2731)}
		if agent.checkStreamStuck(msg) {
			t.Error("has tool calls -> not stuck even above early threshold")
		}
	})
}

func TestCheckStreamStuck_ReasoningModel(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			MaxTokens:       2730,
			ReasoningBudget: 2730, // reasoning model
			SkipStuckCheck:  false,
		},
	}

	t.Run("above early threshold not stuck", func(t *testing.T) {
		msg := &proxy.Message{ReasoningContent: strings.Repeat("x", 1366)}
		if agent.checkStreamStuck(msg) {
			t.Error("reasoning model should skip early check")
		}
	})

	t.Run("above standard threshold stuck", func(t *testing.T) {
		msg := &proxy.Message{ReasoningContent: strings.Repeat("x", 5461)}
		if !agent.checkStreamStuck(msg) {
			t.Error("reasoning model should hit standard stuck threshold")
		}
	})
}

func TestAgent_FallbackXMLModeNoToolChoice(t *testing.T) {
	// When the empty-stream-native-tools fallback fires, the non-streaming
	// retry should use XML mode: no tool_choice:required, no native tools.
	var capturedReq proxy.ChatRequest
	var chatCallCount int
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			chatCallCount++
			capturedReq = req
			if chatCallCount == 1 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "Task complete"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
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

	agent := &Agent{config: AgentConfig{ContextBudget: 36000}, deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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

	agent := &Agent{config: AgentConfig{ContextBudget: 33000}, deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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

	agent := &Agent{config: AgentConfig{ContextBudget: 500}, deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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
			agent := &Agent{config: AgentConfig{ContextBudget: 100}, deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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
	agent := &Agent{deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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
		{Role: proxy.SystemRole, Content: "You are an autonomous agent..."},      // [0] system
		{Role: proxy.SystemRole, Content: "Goal: Execute task\n- [DONE] Step 1"}, // [1] PlanState
		{Role: proxy.UserRole, Content: "TASK: Run the smoke test steps..."},     // [2] task
	}
	for i := 0; i < 24; i++ {
		history = append(history, proxy.Message{Role: proxy.AssistantRole, Content: strings.Repeat("x", 10000)})
	}

	agent := &Agent{config: AgentConfig{ContextBudget: 500}, deps: AgentRuntimeDeps{Logger: logging.NewNopLogger()}}
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

func TestPrepareChatRequest_ZeroReasoningBudget(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "Task completed successfully"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          5,
		MaxResponseTokens: 4096,
		ReasoningBudget:   0, // non-reasoning model — should NOT auto-assign
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " in workspace 'ws-1'.\nExecute the task."},
	}

	req := agent.buildChatRequest(prepared, nil)

	if agent.config.ReasoningBudget != 0 {
		t.Errorf("expected agent.config.ReasoningBudget = 0 for non-reasoning model, got %d", agent.config.ReasoningBudget)
	}
	if req.ReasoningBudget != 0 {
		t.Errorf("expected req.ReasoningBudget = 0 for non-reasoning model, got %d", req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != 0 {
		t.Errorf("expected req.ThinkingBudgetTokens = 0 for non-reasoning model, got %d", req.ThinkingBudgetTokens)
	}
}

func TestPrepareChatRequest_UsesExplicitReasoningBudget(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "Task completed successfully"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	explicitBudget := 1234
	client.ReasoningFieldOverride = proxy.ReasoningFieldThinkTokens
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          5,
		MaxResponseTokens: 4096,
		ReasoningBudget:   explicitBudget,
		ProviderType:      "local",
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " in workspace 'ws-1'.\nExecute the task."},
	}

	req := agent.buildChatRequest(prepared, nil)

	if agent.config.ReasoningBudget != explicitBudget {
		t.Errorf("expected agent.config.ReasoningBudget = %d, got %d", explicitBudget, agent.config.ReasoningBudget)
	}
	if req.ReasoningBudget != 0 {
		t.Errorf("expected req.ReasoningBudget = 0 for local provider (uses thinking_budget_tokens), got %d", req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != explicitBudget {
		t.Errorf("expected req.ThinkingBudgetTokens = %d for local provider, got %d", explicitBudget, req.ThinkingBudgetTokens)
	}
}

func TestPrepareChatRequest_ZeroBudgetHasNoEffect(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "Task completed successfully"}},
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
		{Role: proxy.UserRole, Content: "plain chat message"},
	}

	req := agent.buildChatRequest(prepared, nil)

	if agent.config.ReasoningBudget != 0 {
		t.Errorf("expected agent.config.ReasoningBudget = 0, got %d", agent.config.ReasoningBudget)
	}
	if req.ReasoningBudget != 0 {
		t.Errorf("expected req.ReasoningBudget = 0, got %d", req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != 0 {
		t.Errorf("expected req.ThinkingBudgetTokens = 0, got %d", req.ThinkingBudgetTokens)
	}
}

func TestPrepareChatRequest_NvidiaDoesNotSendThinkingBudgetTokens(t *testing.T) {
	client := &MockClient{
		Response: proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{Role: "assistant", Content: "Task completed successfully"}},
			},
		},
	}
	provider := &MockProvider{}
	engine := &MockEngine{}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          5,
		MaxResponseTokens: 4096,
		ReasoningBudget:   2048,
		ProviderType:      "nvidia",
	})

	prepared := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " in workspace 'ws-1'.\nExecute the task."},
	}

	req := agent.buildChatRequest(prepared, nil)

	if req.ChatTemplateKwargs == nil || !req.ChatTemplateKwargs.EnableThinking {
		t.Errorf("expected Nvidia to send ChatTemplateKwargs.EnableThinking=true, got %+v", req.ChatTemplateKwargs)
	}
	if req.ReasoningBudget != 0 {
		t.Errorf("expected req.ReasoningBudget = 0 for Nvidia, got %d", req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != 0 {
		t.Errorf("Nvidia should NOT receive thinking_budget_tokens, got %d", req.ThinkingBudgetTokens)
	}
}

func TestPrepareChatRequest_CloudProviderDoesNotSendThinkingBudgetTokens(t *testing.T) {
	for _, pt := range []string{"openai", "openrouter", "mulerouter", "gemini", "vertex"} {
		t.Run(pt, func(t *testing.T) {
			client := &MockClient{
				Response: proxy.ChatResponse{
					Choices: []proxy.Choice{
						{Message: proxy.Message{Role: "assistant", Content: "ok"}},
					},
				},
			}

			agent := NewAgent(client, &MockProvider{}, &MockEngine{}, AgentOptions{
				MaxResponseTokens: 4096,
				ReasoningBudget:   8192,
				ProviderType:      pt,
			})

			prepared := []proxy.Message{
				{Role: proxy.SystemRole, Content: "system"},
				{Role: proxy.UserRole, Content: "hello"},
			}

			req := agent.buildChatRequest(prepared, nil)

			if pt == "openrouter" {
				if req.Reasoning == nil {
					t.Errorf("openrouter: expected req.Reasoning object to be set, got nil")
				}
			} else {
				if req.ReasoningEffort == "" {
					t.Errorf("%s: expected req.ReasoningEffort set, got empty", pt)
				}
			}
			if req.ThinkingBudgetTokens != 0 {
				t.Errorf("%s: should NOT send thinking_budget_tokens, got %d", pt, req.ThinkingBudgetTokens)
			}
			if req.ReasoningBudget != 0 {
				t.Errorf("%s: should NOT send flat reasoning_budget, got %d", pt, req.ReasoningBudget)
			}
		})
	}
}

// TestResolveReasoningSpec_LocalAutoBudget verifies the SSOT derivation: a local
// (ModeThinkTokens) provider with no explicit budget derives it from max_tokens
// via DefaultReasoningBudget (max_tokens/3), tying it to the server's serving
// context. No name matching involved.
func TestResolveReasoningSpec_LocalAutoBudget(t *testing.T) {
	spec := resolveReasoningSpec("local", 0, 2730)
	if spec.Mode != ModeThinkTokens {
		t.Fatalf("local should resolve to ModeThinkTokens, got %v", spec.Mode)
	}
	want := DefaultReasoningBudget(2730) // 2730/3 = 910
	if spec.Budget != want {
		t.Errorf("expected auto budget %d (max_tokens/3), got %d", want, spec.Budget)
	}
}

// TestResolveReasoningSpec_LocalExplicitWins verifies an explicit reasoning
// budget overrides the context-derived default.
func TestResolveReasoningSpec_LocalExplicitWins(t *testing.T) {
	spec := resolveReasoningSpec("local", 2048, 2730)
	if spec.Budget != 2048 {
		t.Errorf("explicit budget should win, got %d", spec.Budget)
	}
}

// TestResolveReasoningSpec_CloudUnaffected verifies cloud providers keep their
// tier Mode and are never given a numeric think-token budget.
func TestResolveReasoningSpec_CloudUnaffected(t *testing.T) {
	for _, pt := range []string{"openai", "gemini", "vertex", "openrouter", "mulerouter", "nvidia"} {
		spec := resolveReasoningSpec(pt, 0, 4096)
		if spec.Mode == ModeThinkTokens {
			t.Errorf("%s should not resolve to think-tokens mode", pt)
		}
		if spec.Budget != 0 {
			t.Errorf("%s should not carry a numeric budget, got %d", pt, spec.Budget)
		}
	}
}

// TestResolveReasoningSpec_UnknownFallsBackToEffort verifies an unknown provider
// defaults to the effort mode (no reasoning params sent) rather than crashing.
func TestResolveReasoningSpec_UnknownFallsBackToEffort(t *testing.T) {
	spec := resolveReasoningSpec("does-not-exist", 0, 4096)
	if spec.Mode != ModeEffort {
		t.Errorf("unknown provider should default to ModeEffort, got %v", spec.Mode)
	}
}

func TestProviderTuningDefaults_ReasoningBudgetField(t *testing.T) {
	// The reasoning-budget wire field is resolved from the upstream client
	// contract, not the provider slug. This test verifies the resolution
	// helper applies exactly one field and clears the other.
	cases := []struct {
		field   string
		budget  int
		wantRB  int
		wantTBT int
	}{
		{proxy.ReasoningFieldThinkTokens, 1234, 0, 1234},
		{proxy.ReasoningFieldBudget, 2048, 2048, 0},
		{proxy.ReasoningFieldBudget, 8192, 8192, 0},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			req := proxy.ChatRequest{}
			proxy.SetReasoningBudget(&req, c.field, c.budget)
			if req.ReasoningBudget != c.wantRB {
				t.Errorf("ReasoningBudget = %d, want %d", req.ReasoningBudget, c.wantRB)
			}
			if req.ThinkingBudgetTokens != c.wantTBT {
				t.Errorf("ThinkingBudgetTokens = %d, want %d", req.ThinkingBudgetTokens, c.wantTBT)
			}

			proxy.SetReasoningBudget(&req, c.field, 0)
			if req.ReasoningBudget != 0 || req.ThinkingBudgetTokens != 0 {
				t.Errorf("zeroing left values: rb=%d tbt=%d", req.ReasoningBudget, req.ThinkingBudgetTokens)
			}
		})
	}
}

// TestClientReasoningField_ViaLocal verifies a local llama.cpp client reports
// the thinking_budget_tokens wire field, independent of the config slug.
func TestClientReasoningField_ViaLocal(t *testing.T) {
	c := proxy.NewLLMClientForLocal("http://127.0.0.1:8080", "m", nil, nil)
	if got := c.ReasoningField(); got != proxy.ReasoningFieldThinkTokens {
		t.Fatalf("local client ReasoningField = %q, want %q", got, proxy.ReasoningFieldThinkTokens)
	}

	cloud := proxy.NewLLMClient("https://api.openai.com", "m", nil, nil)
	if got := cloud.ReasoningField(); got != proxy.ReasoningFieldBudget {
		t.Fatalf("cloud client ReasoningField = %q, want %q", got, proxy.ReasoningFieldBudget)
	}
}

func TestAgent_Execute_EventOrder_ToolCallBeforeEventMessage(t *testing.T) {
	var mu sync.Mutex
	var events []AgentEvent
	var chatCallCount int

	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			chatCallCount++
			if chatCallCount == 1 {
				tc := proxy.ToolCall{ID: "call_submit", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path": ".", "summary": "done"}`}}
				msg := proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{tc}}
				return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
			}
			msg := proxy.Message{Role: "assistant", Content: "Task completed successfully"}
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{
			FileSystem: models.FileSystemGuardrailsConfig{
				Enabled:      true,
				AllowedPaths: []string{"."},
			},
		}
	}, storage.NewPathResolver("", "", ""), nil, nil)

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:   5,
		Guardrails: gr,
		Observer: func(ev AgentEvent) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, ev)
		},
	})

	_, _, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "go"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var toolCallIdx, assistantMsgIdx int = -1, -1
	for i, ev := range events {
		if ev.Type == EventToolCall {
			toolCallIdx = i
		}
		if ev.Type == EventMessage {
			msg, ok := ev.Payload.(proxy.Message)
			if ok && msg.Role == "assistant" {
				assistantMsgIdx = i
			}
		}
	}

	if toolCallIdx < 0 {
		t.Fatal("EventToolCall event not found — tool_call event must be emitted")
	}
	if assistantMsgIdx < 0 {
		t.Fatal("assistant EventMessage event not found — message event must be emitted")
	}
	if toolCallIdx >= assistantMsgIdx {
		t.Errorf("EventToolCall (index %d) should appear before assistant EventMessage (index %d) — processToolCalls must notify tool_call before EventMessage", toolCallIdx, assistantMsgIdx)
	}
}

func TestAgent_Execute_NativeToolsNoToolCalls_NagThenSuccess(t *testing.T) {
	// Phase 2b (Hermes-aligned): native-tools freeform text is accepted as
	// completion.  The agent no longer nags when the model writes text without
	// tool calls in native mode — text without tools IS the completion signal.
	var callCount int
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			// First call: text without tool calls in native-tools mode.
			// Hermes-aligned: accepted as completion immediately (no nag).
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", Content: "Task completed successfully."}}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps: 5,
	})

	// History includes a tool result so the any-tool-result gate passes.
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "tell me a joke"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{
			{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path":"joke.txt"}`}},
		}},
		{Role: proxy.ToolRole, Content: "Why did the chicken cross the road?"},
	}

	reply, _, err := agent.Execute(context.Background(), history)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(reply, "Task completed successfully") {
		t.Errorf("expected reply containing 'Task completed successfully.', got %q", reply)
	}
	if callCount != 1 {
		t.Errorf("expected 1 LLM call, got %d (native-tools completion should not nag)", callCount)
	}
}

func TestAgent_Execute_NativeToolsNoToolCalls_Starvation(t *testing.T) {
	// Phase 2b (Hermes-aligned): text without tool calls is accepted as
	// completion.  Starvation (repeated empty/text turns) still triggers
	// after DefaultStarvationLimit empty turns, but a single substantive
	// text answer completes immediately rather than escalating to starvation.
	var callCount int
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			// Return empty content with reasoning — avoids premature
			// termination (reasoning-only is not premature) and reaches
			// one-shot nag, then forced completion.
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:             "assistant",
						Content:          "",
						ReasoningContent: "still thinking...",
					}},
				},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      25,
		ContextBudget: 100000,
	})

	// History includes a tool result so we test post-work empty response.
	// Empty content with ReasoningContent avoids premature termination
	// (reasoning-only is NOT premature) and triggers the one-shot nag.
	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "do something"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{
			{ID: "c1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path":"task.md"}`}},
		}},
		{Role: proxy.ToolRole, Content: "data"},
	})
	// Phase 2b: empty content after tools triggers one-shot nag, then
	// premature termination (isPrematureTermination).  This exits
	// cleanly — no starvation error, just empty completion.
	if err != nil {
		t.Fatalf("unexpected error: %v (empty after tools should complete, not starve)", err)
	}
	if callCount <= 1 {
		t.Errorf("expected at least 2 calls (empty + one-shot nag), got %d", callCount)
	}
}

// TestAgent_Execute_ReasoningOnlyWriteThenEmpty_Terminates is a regression test
// for the unattended-run safety-hardening regression where a reasoning-only
// model (content_len 0 every turn) wrote its deliverable via write_file and
// then returned empty reasoning-only turns. The bounded recovery ladder must
// terminate the run without re-scanning: postToolNudgeCount re-arms on every
// successful tool turn, the nudge fires up to postToolNudgeMax times, then a
// single tools-disabled finalization turn is forced, and finally the run ends.
//
// Because the write is a COMPLETED (valid-JSON) call, its content is NOT dumped
// as the final report (that would misreport a source/artifact file); the final
// reply is empty since the model never emitted a chat report.
func TestAgent_Execute_ReasoningOnlyWriteThenEmpty_Terminates(t *testing.T) {
	var callCount int
	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return &proxy.ChatResponse{
					Choices: []proxy.Choice{
						{Message: proxy.Message{
							Role:    "assistant",
							Content: "",
							ToolCalls: []proxy.ToolCall{
								{ID: "c1", Type: "function", Function: proxy.FunctionCall{
									Name:      models.ToolFileWrite,
									Arguments: `{"path":"report.md","content":"# Network Recon Report\nThe local network was scanned in two phases. Five active hosts were discovered. SSH was exposed on three hosts and HTTP on two. One host also exposed SMB and RDP services that should be firewalled. This report documents the open ports, the detected service versions, and the recommended hardening steps to reduce the attack surface."}`,
								}},
							},
						}},
					},
				}, nil
			}
			// Turn 2+: empty reasoning-only. Must end the run, not re-scan.
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{Role: "assistant", Content: "", ReasoningContent: "thinking"}},
				},
			}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: models.ToolFileWrite}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		UseNativeTools: boolPtr(true),
		MaxSteps:       25,
		ContextBudget:  100000,
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "scan the network and write a report"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The model completed its work via a write_file tool call and then returned
	// empty reasoning-only turns. The new bounded ladder re-arms the nudge up to
	// postToolNudgeMax times, forces ONE tools-disabled finalization turn, then
	// terminates — it must NOT re-scan or loop forever. Because the model never
	// emitted a chat report (only a tool call), the final reply is empty.
	if callCount < 2 {
		t.Errorf("expected at least the write turn + a recovery turn, got %d", callCount)
	}
	if callCount > 8 {
		t.Errorf("run did not terminate (re-armed nudge loop?), got %d calls", callCount)
	}
	if reply != "" {
		// If a reply is produced it must be a genuine report, never a tool marker.
		if hasToolCallMarker(reply) {
			t.Errorf("reply must not contain tool-call markers, got %q", reply)
		}
	}
}

// boolPtr is defined earlier in this file (line ~2545)

func TestExceedsContentCharCap(t *testing.T) {
	agent := &Agent{config: AgentConfig{MaxTokens: 2730}}

	t.Run("under cap", func(t *testing.T) {
		msg := &proxy.Message{Content: strings.Repeat("x", 10000)}
		if agent.exceedsContentCharCap(msg) {
			t.Error("10000 chars should be under cap (2730*4=10920)")
		}
	})

	t.Run("at cap not exceeded", func(t *testing.T) {
		msg := &proxy.Message{Content: strings.Repeat("x", 10920)}
		if agent.exceedsContentCharCap(msg) {
			t.Error("10920 chars should be at cap, not over (cap is 4*2730=10920)")
		}
	})

	t.Run("over cap", func(t *testing.T) {
		msg := &proxy.Message{Content: strings.Repeat("x", 10921)}
		if !agent.exceedsContentCharCap(msg) {
			t.Error("10921 chars should exceed cap")
		}
	})

	t.Run("zero maxTokens disables", func(t *testing.T) {
		zero := &Agent{config: AgentConfig{MaxTokens: 0}}
		msg := &proxy.Message{Content: strings.Repeat("x", 99999)}
		if zero.exceedsContentCharCap(msg) {
			t.Error("zero maxTokens should disable cap")
		}
	})
}

func TestAgent_CancelDuringStream_NoFallbackToNonStreaming(t *testing.T) {
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, context.Canceled
		},
		// No ChatFunc — fallback would call this; should NOT be reached.
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			t.Error("non-streaming ChatFunc was called; fallback should be skipped on cancel")
			return nil, fmt.Errorf("should not reach")
		},
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _, err := agent.Execute(ctx, []proxy.Message{{Role: "user", Content: "stop me"}})

	if err == nil {
		t.Fatal("expected error from canceled stream, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected error wrapping context.Canceled, got: %v", err)
	}
}

func TestAgent_CancelDuringStreamXMLRetry_NoFallbackToNonStreaming(t *testing.T) {
	// First stream call returns an error to trigger XML retry path.
	// The XML retry also returns context.Canceled.  Must NOT fall back
	// to non-streaming.
	streamCallCount := 0
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCallCount++
			return nil, context.Canceled
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			t.Error("non-streaming ChatFunc was called; fallback should be skipped on cancel")
			return nil, fmt.Errorf("should not reach")
		},
	}
	agent := NewAgent(client, &MockProvider{}, &MockEngine{}, AgentOptions{MaxSteps: 5, UseNativeTools: boolPtr(false)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _, err := agent.Execute(ctx, []proxy.Message{{Role: "user", Content: "stop me"}})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected error wrapping context.Canceled, got: %v", err)
	}
	if streamCallCount == 0 {
		t.Error("expected at least one stream call")
	}
}

func TestAgent_SubmitFinalAnswer_NoDuplicateInHistory(t *testing.T) {
	var events []AgentEvent

	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			return nil, fmt.Errorf("streaming not supported")
		},
	}
	client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
		if client.Calls == 1 {
			// Turn 1: a real tool call, producing a tool result.
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{
					{Message: proxy.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []proxy.ToolCall{{
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      "read_file",
								Arguments: `{"path": "report.txt"}`,
							},
						}},
					}},
				},
			}, nil
		}
		// Turn 2: content-only final answer after the tool result.
		return &proxy.ChatResponse{
			Choices: []proxy.Choice{
				{Message: proxy.Message{
					Role:    "assistant",
					Content: "Workspace report generated with 6 files",
				}},
			},
		}, nil
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}

	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:      5,
		GlobalTimeout: 30 * time.Second,
	})
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

	reply, history, err := agent.Execute(context.Background(), []proxy.Message{{Role: "user", Content: "list all files"}})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(reply, "Workspace report generated") {
		t.Errorf("expected reply to contain 'Workspace report generated', got '%s'", reply)
	}

	lastMsg := history[len(history)-1]
	if lastMsg.Role != proxy.AssistantRole {
		t.Errorf("expected last message role to be assistant, got %s", lastMsg.Role)
	}
	if lastMsg.Content != "Workspace report generated with 6 files" {
		t.Errorf("expected last assistant message to contain the report, got %q", lastMsg.Content)
	}

	// There must be exactly one assistant message carrying the report content
	// (the final answer). No spurious duplicate from a synthetic submit tool.
	assistantReportCount := 0
	for _, m := range history {
		if m.Role == proxy.AssistantRole && m.Content == "Workspace report generated with 6 files" {
			assistantReportCount++
		}
	}
	if assistantReportCount != 1 {
		t.Errorf("expected exactly 1 assistant message with the report, got %d", assistantReportCount)
	}

	// Verify the EventMessage SSE event carries the report in its content field.
	var msgEventContent string
	for _, ev := range events {
		if ev.Type == EventMessage {
			if msg, ok := ev.Payload.(proxy.Message); ok && msg.Role == proxy.AssistantRole && strings.Contains(msg.Content, "Workspace report generated") {
				msgEventContent = msg.Content
				break
			}
		}
	}
	if msgEventContent == "" {
		t.Error("expected an EventMessage carrying the final report content")
	}
}

// TestAgent_ReasoningAndContentSeparateStreams verifies that reasoning content
// and response content are streamed via separate event types:
//   - EventReasoning for reasoning_content chunks (the model's thinking)
//   - EventToolStream for content chunks (the visible response)
//
// This prevents the UI from showing reasoning and response as duplicated blocks
// while still giving users visibility into the agent's planning.
func TestAgent_ReasoningAndContentSeparateStreams(t *testing.T) {
	const reasoningText = "The user wants me to perform a network scan. " +
		"I will start with Phase 1 rapid host discovery. "
	const contentText = "I'll begin by scanning the local network for active hosts. " +
		"Let me call scan_local_network with mode fast. "

	var events []AgentEvent
	var streamCallCount int
	client := &MockClient{
		StreamFunc: func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
			streamCallCount++
			ch := make(chan *proxy.ChatResponse, 100)
			go func() {
				defer close(ch)
				if streamCallCount == 1 {
					// Phase 1: reasoning chunks only (no content yet).
					for _, c := range splitIntoChunks(reasoningText, 12) {
						ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
							Delta: proxy.Message{ReasoningContent: c},
						}}}
					}
					// Phase 2: content chunks only.
					for _, c := range splitIntoChunks(contentText, 12) {
						ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
							Delta: proxy.Message{Content: c},
						}}}
					}
					// Final: a tool call to end the turn.
					ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{
						Delta: proxy.Message{ToolCalls: []proxy.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      "read_file",
								Arguments: `{"summary": "done"}`,
							},
						}}},
					}}}
					return
				}
				// Second stream call (after tool execution): content-only signals completion
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "Task completed successfully"}}}}
			}()
			return ch, nil
		},
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: proxy.Message{Role: "assistant", ToolCalls: []proxy.ToolCall{{ID: "call_1", Type: "function", Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"summary": "done"}`}}}}}}}, nil
		},
	}
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: proxy.FunctionSchema{Name: "read_file"}},
		},
	}
	engine := &MockEngine{Result: "ok"}
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:          25,
		MaxResponseTokens: 2730,
		ReasoningBudget:   2730,
	})
	agent.deps.Observer = func(ev AgentEvent) { events = append(events, ev) }

	_, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: prompts.AutomationMarker + " do the task"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var reasoningCount, toolStreamCount int
	var lastReasoningPayload string
	for _, ev := range events {
		switch ev.Type {
		case EventReasoning:
			reasoningCount++
			lastReasoningPayload, _ = ev.Payload.(string)
		case EventToolStream:
			toolStreamCount++
			s, _ := ev.Payload.(string)
			if strings.Contains(s, "network scan") || strings.Contains(s, "Phase 1") {
				t.Errorf("reasoning text leaked into EventToolStream: %q", s)
			}
		}
	}

	if reasoningCount == 0 {
		t.Error("expected at least one EventReasoning event for model thinking")
	}
	if toolStreamCount == 0 {
		t.Error("expected at least one EventToolStream event for response content")
	}
	if lastReasoningPayload != reasoningText {
		t.Errorf("last EventReasoning payload = %q, want %q", lastReasoningPayload, reasoningText)
	}
}

// splitIntoChunks splits s into chunks of at most size runes for streaming tests.
func splitIntoChunks(s string, size int) []string {
	var out []string
	runes := []rune(s)
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	return out
}

// TestAgent_Execute_TruncatedWriteFile_Salvages ensures native tool_calls with
// truncated write_file JSON complete via the unified salvage path (Path A).
// Content-only args (no path) complete without calling Engine.
func TestAgent_Execute_TruncatedWriteFile_Salvages(t *testing.T) {
	reportBody := strings.Repeat("n", salvageMinContentLen) + "\n# Network Report\nfindings tail"
	// Unterminated content string — same class as production native failure.
	truncArgs := `{"content":"` + reportBody

	client := &MockClient{
		ChatFunc: func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
			return &proxy.ChatResponse{
				Choices: []proxy.Choice{{
					Message: proxy.Message{
						Role:    proxy.AssistantRole,
						Content: "Compiling the Markdown report.",
						ToolCalls: []proxy.ToolCall{{
							ID:   "call_write",
							Type: "function",
							Function: proxy.FunctionCall{
								Name:      models.ToolFileWrite,
								Arguments: truncArgs,
							},
						}},
					},
				}},
			}, nil
		},
	}
	native := true
	provider := &MockProvider{
		UseNative: &native,
		Tools: []proxy.Tool{{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name: models.ToolFileWrite,
				Parameters: map[string]any{
					"type":     "object",
					"required": []any{"path", "content"},
					"properties": map[string]any{
						"path":    map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
					},
				},
			},
		}},
	}
	engine := &MockEngine{Result: "should not execute"}
	agent := NewAgent(client, provider, engine, AgentOptions{
		MaxSteps:       5,
		UseNativeTools: &native,
	})

	reply, _, err := agent.Execute(context.Background(), []proxy.Message{
		{Role: proxy.UserRole, Content: "Run network scan and produce a Markdown report."},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(reply, "Network Report") || !strings.Contains(reply, "findings tail") {
		t.Fatalf("expected salvaged report in reply, got %q", reply[:min(120, len(reply))])
	}
	if engine.Calls != 0 {
		t.Fatalf("engine must not run without recoverable path, calls=%d", engine.Calls)
	}
}

func TestToolCache_RebuildAndReuse(t *testing.T) {
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: models.FunctionSchema{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"path": "string"}}},
		},
	}
	agent := NewAgent(&MockClient{}, provider, &MockEngine{}, AgentOptions{MaxSteps: 5})

	agent.rebuildToolCache(context.Background())

	if agent.cachedToolManual == "" {
		t.Fatal("expected cachedToolManual to be populated after rebuild")
	}
	if agent.cachedToolReference == "" {
		t.Fatal("expected cachedToolReference to be populated after rebuild")
	}
	if agent.toolsHash == 0 {
		t.Fatal("expected toolsHash to be non-zero after rebuild")
	}
	firstHash := agent.toolsHash
	firstManual := agent.cachedToolManual
	firstReference := agent.cachedToolReference

	agent.rebuildToolCache(context.Background())

	if agent.toolsHash != firstHash {
		t.Errorf("toolsHash changed after same tools: %d -> %d", firstHash, agent.toolsHash)
	}
	if agent.cachedToolManual != firstManual {
		t.Error("cachedToolManual was rebuilt when tools did not change")
	}
	if agent.cachedToolReference != firstReference {
		t.Error("cachedToolReference was rebuilt when tools did not change")
	}
}

func TestToolCache_InvalidatesOnDifferentTools(t *testing.T) {
	provider := &MockProvider{
		Tools: []proxy.Tool{
			{Type: "function", Function: models.FunctionSchema{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"path": "string"}}},
		},
	}
	agent := NewAgent(&MockClient{}, provider, &MockEngine{}, AgentOptions{MaxSteps: 5})

	agent.rebuildToolCache(context.Background())
	firstHash := agent.toolsHash
	firstManual := agent.cachedToolManual

	provider.Tools = append(provider.Tools, proxy.Tool{
		Type: "function", Function: models.FunctionSchema{Name: "grep", Description: "Search files", Parameters: map[string]any{"pattern": "string"}},
	})
	agent.rebuildToolCache(context.Background())

	if agent.toolsHash == firstHash {
		t.Error("toolsHash should have changed after tools changed")
	}
	if agent.cachedToolManual == firstManual {
		t.Error("cachedToolManual should have been rebuilt")
	}
}

func TestToolCtxWithTimeout_FilesystemUsesFilesystemTimeout(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		ToolTimeout:           2 * time.Minute,
		FilesystemToolTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolFileRead)
	defer cancel()

	deadline, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("expected filesystem ctx to have deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 29*time.Second || remaining > 30*time.Second+50*time.Millisecond {
		t.Errorf("filesystem timeout ~30s, got remaining=%v", remaining)
	}
}

func TestToolCtxWithTimeout_TerminalUsesDefaultTimeout(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		ToolTimeout:           2 * time.Minute,
		FilesystemToolTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolTerminalExecute)
	defer cancel()

	deadline, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("expected terminal ctx to have deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 1*time.Minute+55*time.Second || remaining > 2*time.Minute+50*time.Millisecond {
		t.Errorf("terminal timeout ~2m, got remaining=%v", remaining)
	}
}

func TestToolCtxWithTimeout_NetworkUsesDefaultTimeout(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		ToolTimeout:           2 * time.Minute,
		FilesystemToolTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolNetworkFetch)
	defer cancel()

	_, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("expected network ctx to have deadline")
	}
}

func TestToolCtxWithTimeout_SearchUsesDefaultTimeout(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		ToolTimeout:           2 * time.Minute,
		FilesystemToolTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolInternetSearch)
	defer cancel()

	_, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("expected search ctx to have deadline")
	}
}

func TestToolCtxWithTimeout_UnknownToolUsesDefaultTimeout(t *testing.T) {
	agent := NewAgent(&MockClient{}, &MockProvider{}, &MockEngine{}, AgentOptions{
		ToolTimeout:           2 * time.Minute,
		FilesystemToolTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, "unknown_tool")
	defer cancel()

	_, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("expected unknown tool ctx to have deadline")
	}
}

func TestToolCtxWithTimeout_ZeroTimeoutReturnsOriginalContext(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			ToolTimeout:           0,
			FilesystemToolTimeout: 0,
		},
	}

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolFileWrite)
	defer cancel()

	_, ok := toolCtx.Deadline()
	if ok {
		t.Errorf("zero timeout should not set deadline, but got one")
	}

	if toolCtx != ctx {
		t.Error("zero timeout should return original context")
	}
}

func TestToolCtxWithTimeout_OnlyFilesystemTimeoutZeroFallsBackToDefault(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			ToolTimeout:           2 * time.Minute,
			FilesystemToolTimeout: 0,
		},
	}

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolFileRead)
	defer cancel()

	deadline, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("filesystem should use ToolTimeout when FilesystemToolTimeout is 0")
	}
	remaining := time.Until(deadline)
	if remaining < 1*time.Minute+55*time.Second || remaining > 2*time.Minute+50*time.Millisecond {
		t.Errorf("filesystem fallback timeout ~2m, got remaining=%v", remaining)
	}
}

func TestToolCtxWithTimeout_ContextCancelFuncIsNoopForZeroTimeout(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			ToolTimeout: 0,
		},
	}

	ctx := context.Background()
	_, cancel := agent.toolCtxWithTimeout(ctx, "some_tool")
	cancel()
}

func TestToolCtxWithTimeout_FilesystemTimeoutNonZeroOverridesDefault(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			ToolTimeout:           2 * time.Minute,
			FilesystemToolTimeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	toolCtx, cancel := agent.toolCtxWithTimeout(ctx, models.ToolFileAppend)
	defer cancel()

	deadline, ok := toolCtx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 9*time.Second || remaining > 10*time.Second+50*time.Millisecond {
		t.Errorf("filesystem timeout ~10s, got remaining=%v", remaining)
	}
}

func TestEventStream_UsesCounterID(t *testing.T) {
	var ids []string
	agent := &Agent{
		deps: AgentRuntimeDeps{
			Logger: logging.NewNopLogger(),
			Observer: func(e AgentEvent) {
				ids = append(ids, e.ID)
			},
		},
	}

	for i := 0; i < 5; i++ {
		agent.notifyStepStart(i + 1)
	}

	if len(ids) != 5 {
		t.Fatalf("expected 5 events, got %d", len(ids))
	}
	// IDs come from a process-global monotonic counter, so assert they are
	// strictly increasing by 1 and all unique (not a fixed starting value).
	seen := make(map[string]bool)
	var prev uint64
	for i, id := range ids {
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			t.Fatalf("event %d: ID %q is not a numeric counter value", i, id)
		}
		if seen[id] {
			t.Errorf("event %d: ID %q is not unique across the stream", i, id)
		}
		seen[id] = true
		if i > 0 && n != prev+1 {
			t.Errorf("event %d: ID %q not exactly 1 greater than previous %d", i, id, prev)
		}
		prev = n
	}
}
