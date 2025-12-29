package mocks

import (
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
)

type MockAssistantService struct {
	DeviceCtx   device_context.DeviceContextProvider
	Client      proxy.LLMClientProvider
	RateLimiter ratelimiter.Limiter
	LoggerRef   logging.Logger
	Model       string
}

func (m *MockAssistantService) DeviceContextProvider() device_context.DeviceContextProvider {
	return m.DeviceCtx
}

func (m *MockAssistantService) ClientProvider() proxy.LLMClientProvider {
	return m.Client
}

func (m *MockAssistantService) Limiter() ratelimiter.Limiter {
	return m.RateLimiter
}

func (m *MockAssistantService) Logger() logging.Logger {
	return m.LoggerRef
}

func (m *MockAssistantService) DefaultModel() (string, error) {
	return m.Model, nil
}
