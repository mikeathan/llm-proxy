package mocks

import (
	"context"
	"errors"
	"time"

	"llm-proxy/internal/core/proxy"
)

// Mock TokenManager
type MockTokenManager struct {
	token     string
	err       error
	callCount int
}

func NewMockTokenManager(token string, err error) *MockTokenManager {
	return &MockTokenManager{
		token: token,
		err:   err,
	}
}

func (m *MockTokenManager) CallCount() int {
	return m.callCount
}

func (m *MockTokenManager) SetToken(token string) {
	m.token = token
}

func (m *MockTokenManager) SetError(err error) {
	m.err = err
}

func (m *MockTokenManager) Get(ctx context.Context) (string, error) {
	m.callCount++
	return m.token, m.err
}

// Mock NodeHerder
type MockNodeHerder struct {
	// Existing fields...

	err          error
	systemPrompt string
	callCount    int

	// New fields for MCPService
	toolsResult []proxy.Tool
	callResult  any
	callErr     error
}

func NewMockNodeHerder(err error) *MockNodeHerder {
	return &MockNodeHerder{
		err:          err,
		systemPrompt: "Mock System Prompt with RULES:", // Default with RULES to satisfy some old tests if any
	}
}

func (m *MockNodeHerder) SetSystemPrompt(prompt string) {
	m.systemPrompt = prompt
}

func (m *MockNodeHerder) SetToolsResult(tools []proxy.Tool) {
	m.toolsResult = tools
}

func (m *MockNodeHerder) SetCallToolResult(result any, err error) {
	m.callResult = result
	m.callErr = err
}

func (m *MockNodeHerder) GetSystemPrompt() (string, error) {
	return m.systemPrompt, nil
}

func (m *MockNodeHerder) CallCount() int {
	return m.callCount
}

func (m *MockNodeHerder) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	m.callCount++
	return m.toolsResult, m.err
}

func (m *MockNodeHerder) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	m.callCount++
	if m.callErr != nil {
		return nil, m.callErr
	}
	// Fail if general error set
	if m.err != nil {
		return nil, m.err
	}
	return m.callResult, nil
}

// Mock LLMClientProvider
type MockLLMClientProvider struct {
	Client       proxy.Client
	GetClientErr error
}

func (m *MockLLMClientProvider) GetClient(ctx context.Context) (proxy.Client, error) {
	if m.GetClientErr != nil {
		return nil, m.GetClientErr
	}
	if m.Client != nil {
		return m.Client, nil
	}
	return nil, errors.New("not implemented")
}

func (m *MockLLMClientProvider) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return m.GetClient(ctx)
}

// Mock LLMClient
type MockLLMClient struct {
	Response  proxy.ChatResponse
	Responses []proxy.ChatResponse
	Err       error
	LastReq   *proxy.ChatRequest
	Requests  []proxy.ChatRequest
	Calls     int
}

func (m *MockLLMClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	copyReq := req
	m.LastReq = &copyReq
	m.Requests = append(m.Requests, copyReq)
	m.Calls++
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.Responses) > 0 {
		idx := m.Calls - 1
		if idx < len(m.Responses) {
			response := m.Responses[idx]
			return &response, nil
		}
		response := m.Responses[len(m.Responses)-1]
		return &response, nil
	}
	response := m.Response
	return &response, nil
}

// Mock RateLimiter
type MockRateLimiter struct{}

func (m *MockRateLimiter) Allow(key string, interval time.Duration) bool { return true }
func (m *MockRateLimiter) Clear()                                        {}
