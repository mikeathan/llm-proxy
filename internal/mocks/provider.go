package mocks

import (
	"context"
	"errors"
	"time"

	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
)

// Mock HttpNodeHerderFetcher
type MockHttpNodeHerderFetcher struct {
	callCount     int
	deviceResult  *nodeherder.DeviceContextResponse
	metricsResult *nodeherder.MetricsQueryResponse
	err           error
}

func NewMockHttpNodeHerderFetcher(result *nodeherder.DeviceContextResponse, err error) *MockHttpNodeHerderFetcher {
	return &MockHttpNodeHerderFetcher{
		deviceResult: result,
		err:          err,
	}
}
func (m *MockHttpNodeHerderFetcher) CallCount() int {
	return m.callCount
}

func (m *MockHttpNodeHerderFetcher) SetDeviceResult(result *nodeherder.DeviceContextResponse) {
	m.deviceResult = result
}

func (m *MockHttpNodeHerderFetcher) SetMetricsResult(result *nodeherder.MetricsQueryResponse) {
	m.metricsResult = result
}

func (m *MockHttpNodeHerderFetcher) FetchDeviceContext() (*nodeherder.DeviceContextResponse, error) {
	m.callCount++
	return m.deviceResult, m.err
}

func (m *MockHttpNodeHerderFetcher) QueryMetrics(ctx context.Context, req *nodeherder.MetricsQueryRequest) (*nodeherder.MetricsQueryResponse, error) {
	m.callCount++
	return m.metricsResult, m.err
}

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
	deviceCtx     *nodeherder.LLMDeviceContext
	metricsResult *nodeherder.MetricsQueryResponse

	err        error
	deviceErr  error
	metricsErr error
	callCount  int
}

func NewMockNodeHerder(err error) *MockNodeHerder {
	return &MockNodeHerder{
		err: err,
	}
}

func (m *MockNodeHerder) SetDeviceContextResult(result *nodeherder.LLMDeviceContext) {
	m.deviceCtx = result
}

func (m *MockNodeHerder) SetDeviceContextError(err error) {
	m.deviceErr = err
}

func (m *MockNodeHerder) SetMetricsResult(result *nodeherder.MetricsQueryResponse) {
	m.metricsResult = result
}

func (m *MockNodeHerder) SetMetricsError(err error) {
	m.metricsErr = err
}
func (m *MockNodeHerder) GetDeviceContext() (*nodeherder.LLMDeviceContext, error) {
	m.callCount++
	if m.deviceErr != nil {
		return m.deviceCtx, m.deviceErr
	}
	return m.deviceCtx, m.err
}

func (m *MockNodeHerder) CallCount() int {
	return m.callCount
}

func (m *MockNodeHerder) QueryMetrics(ctx context.Context, req *nodeherder.MetricsQueryRequest) (*nodeherder.MetricsQueryResponse, error) {
	m.callCount++
	if m.metricsErr != nil {
		return m.metricsResult, m.metricsErr
	}
	return m.metricsResult, m.err
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
