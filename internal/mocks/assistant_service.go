package mocks

import (
	"llm-proxy/internal/assistant"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
)

type MockAssistantService struct {
	Herder      nodeherder.NodeHerderService
	Client      proxy.LLMClientProvider
	RateLimiter ratelimiter.Limiter
	LoggerRef   logging.Logger
	Model       string
	EngineRef   assistant.Engine
}

func (m *MockAssistantService) NodeHerder() nodeherder.NodeHerderService {
	return m.Herder
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

func (m *MockAssistantService) Engine() assistant.Engine {
	if m.EngineRef != nil {
		return m.EngineRef
	}
	return assistant.NewEngine(m.Herder, m.LoggerRef)
}
