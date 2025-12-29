package mocks

import (
	"context"
	"errors"
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/proxy"
	"time"
)

// Mock HttpDeviceContextFetcher
type MockHttpDeviceContextFetcher struct {
	callCount int
	result    *device_context.DeviceContextResponse
	err       error
}

func NewMockHttpDeviceContextFetcher(result *device_context.DeviceContextResponse, err error) *MockHttpDeviceContextFetcher {
	return &MockHttpDeviceContextFetcher{
		result: result,
		err:    err,
	}
}
func (m *MockHttpDeviceContextFetcher) CallCount() int {
	return m.callCount
}

func (m *MockHttpDeviceContextFetcher) SetResult(result *device_context.DeviceContextResponse) {
	m.result = result
}

func (m *MockHttpDeviceContextFetcher) FetchDeviceContext() (*device_context.DeviceContextResponse, error) {
	m.callCount++
	return m.result, m.err
}

// Mock DeviceContextProvider
type MockDeviceContextProvider struct {
	ctx       *device_context.LLMDeviceContext
	err       error
	callCount int
}

func NewMockDeviceContextProvider(ctx *device_context.LLMDeviceContext, err error) *MockDeviceContextProvider {
	return &MockDeviceContextProvider{
		ctx: ctx,
		err: err,
	}
}
func (m *MockDeviceContextProvider) GetDeviceContext() (*device_context.LLMDeviceContext, error) {
	m.callCount++
	return m.ctx, m.err
}

func (l *MockDeviceContextProvider) CallCount() int {
	return l.callCount
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
