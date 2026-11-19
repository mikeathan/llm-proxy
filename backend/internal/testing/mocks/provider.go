package mocks

import (
	"context"
	"errors"
	"time"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
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

// MockDynamicTools stands in for the tools a stack offers at run time.
type MockDynamicTools struct {
	err       error
	callCount int

	toolsResult []proxy.Tool
	callResult  any
	callErr     error
}

func NewMockDynamicTools(err error) *MockDynamicTools {
	return &MockDynamicTools{err: err}
}

func (m *MockDynamicTools) SetToolsResult(tools []proxy.Tool) {
	m.toolsResult = tools
}

func (m *MockDynamicTools) SetCallToolResult(result any, err error) {
	m.callResult = result
	m.callErr = err
}

func (m *MockDynamicTools) CallCount() int {
	return m.callCount
}

func (m *MockDynamicTools) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	m.callCount++
	return m.toolsResult, m.err
}

func (m *MockDynamicTools) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
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

func (m *MockDynamicTools) UseNativeTools() bool {
	return true
}

// Policy: the double offers no tool that asks first.
func (m *MockDynamicTools) Policy(string) (managed, ask bool) { return false, false }

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

// ColdStartProvider reports a cold model (models.ErrModelStarting) for the first Starting calls, then a ready client:
// Ready, or an empty MockLLMClient.
type ColdStartProvider struct {
	Starting int
	Ready    proxy.Client
	calls    int
}

// Calls is how many times a client was asked for.
func (p *ColdStartProvider) Calls() int { return p.calls }

func (p *ColdStartProvider) GetClient(context.Context) (proxy.Client, error) {
	p.calls++
	if p.calls <= p.Starting {
		return nil, models.ErrModelStarting
	}
	if p.Ready != nil {
		return p.Ready, nil
	}
	return &MockLLMClient{}, nil
}

func (p *ColdStartProvider) GetClientForModel(ctx context.Context, _ string) (proxy.Client, error) {
	return p.GetClient(ctx)
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

func (m *MockLLMClient) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	return nil, errors.New("streaming not implemented in mock")
}

// ReasoningField returns the default OpenAI-compatible wire field.
func (m *MockLLMClient) ReasoningField() string { return proxy.ReasoningFieldBudget }

// Mock RateLimiter
type MockRateLimiter struct{}

func (m *MockRateLimiter) Allow(key string, interval time.Duration) bool { return true }
func (m *MockRateLimiter) Clear()                                        {}
