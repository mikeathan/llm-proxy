package mocks

import (
	"llm-proxy/internal/assistant"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
)

// noopLogger for testing
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...any)   {}
func (l *noopLogger) Info(msg string, args ...any)    {}
func (l *noopLogger) Warn(msg string, args ...any)    {}
func (l *noopLogger) Error(msg string, args ...any)   {}
func (l *noopLogger) With(args ...any) logging.Logger { return l }
func (l *noopLogger) SetLevel(logging.Level)          {}
func (l *noopLogger) Level() logging.Level            { return logging.LevelInfo }

type MockAssistantService struct {
	Herder      nodeherder.MCPService
	Client      proxy.LLMClientProvider
	RateLimiter ratelimiter.Limiter
	LoggerRef   logging.Logger
	Model       string
	EngineRef   assistant.Engine
}

func (m *MockAssistantService) NodeHerder() nodeherder.MCPService {
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

func NewMockAssistantService(
	client proxy.LLMClientProvider,
	limiter ratelimiter.Limiter,
	engine assistant.Engine,
	herder nodeherder.MCPService,
) *MockAssistantService {
	return &MockAssistantService{
		Client:      client,
		RateLimiter: limiter,
		EngineRef:   engine,
		Herder:      herder,
		LoggerRef:   &noopLogger{},
	}
}
