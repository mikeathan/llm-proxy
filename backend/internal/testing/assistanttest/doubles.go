// Package assistanttest holds the test doubles the agent loop's tests drive it with. It imports only proxy and the
// platform memory store, so a test in package assistant (or any package assistant imports) can use it without an
// import cycle.
package assistanttest

import (
	"context"
	"fmt"

	"llm-proxy/internal/core/proxy"
)

// MockClient implements proxy.Client.
type MockClient struct {
	Response               proxy.ChatResponse
	Err                    error
	Calls                  int
	ChatFunc               func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error)
	StreamFunc             func(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error)
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

// MockProvider implements the assistant's ToolProvider.
type MockProvider struct {
	Tools     []proxy.Tool
	UseNative *bool // nil = true (backward compat)
}

func (m *MockProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	return m.Tools, nil
}

func (m *MockProvider) UseNativeTools() bool {
	if m.UseNative == nil {
		return true
	}
	return *m.UseNative
}

// MockEngine implements the assistant's Engine.
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
