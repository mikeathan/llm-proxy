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
	callCount int
	result    *nodeherder.DeviceContextResponse
	err       error
}

func NewMockHttpNodeHerderFetcher(result *nodeherder.DeviceContextResponse, err error) *MockHttpNodeHerderFetcher {
	return &MockHttpNodeHerderFetcher{
		result: result,
		err:    err,
	}
}
func (m *MockHttpNodeHerderFetcher) CallCount() int {
	return m.callCount
}

func (m *MockHttpNodeHerderFetcher) SetResult(result *nodeherder.DeviceContextResponse) {
	m.result = result
}

func (m *MockHttpNodeHerderFetcher) FetchDeviceContext() (*nodeherder.DeviceContextResponse, error) {
	m.callCount++
	return m.result, m.err
}

// Mock NodeHerder
type MockNodeHerder struct {
	ctx       *nodeherder.LLMDeviceContext
	err       error
	callCount int
}

func NewMockNodeHerder(ctx *nodeherder.LLMDeviceContext, err error) *MockNodeHerder {
	return &MockNodeHerder{
		ctx: ctx,
		err: err,
	}
}
func (m *MockNodeHerder) GetDeviceContext() (*nodeherder.LLMDeviceContext, error) {
	m.callCount++
	return m.ctx, m.err
}

func (m *MockNodeHerder) CallCount() int {
	return m.callCount
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
	Response proxy.ChatResponse
	Err      error
}

func (m *MockLLMClient) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	response := m.Response
	return &response, nil
}

// Mock RateLimiter
type MockRateLimiter struct{}

func (m *MockRateLimiter) Allow(key string, interval time.Duration) bool { return true }
func (m *MockRateLimiter) Clear()                                        {}
