package mocks

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
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
	Herder         nodeherder.MCPService
	Client         proxy.LLMClientProvider
	RateLimiter    ratelimiter.Limiter
	LoggerRef      logging.Logger
	Model          string
	EngineRef      assistant.Engine
	PersistenceMgr *persistence.WorkspaceManager
	EventBusRef    *automation.EventBus
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

func (m *MockAssistantService) SelectModels() (string, string) {
	return "", ""
}

func (m *MockAssistantService) Engine() assistant.Engine {
	if m.EngineRef != nil {
		return m.EngineRef
	}
	return assistant.NewEngine(m.Herder, m.LoggerRef)
}

func (m *MockAssistantService) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return m.Client.GetClientForModel(ctx, modelName)
}

func (m *MockAssistantService) GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error) {
	return nil, nil
}

func (m *MockAssistantService) ModelConfig(modelName string) (models.ModelConfig, bool) {
	return models.ModelConfig{}, false
}

func (m *MockAssistantService) GuardrailEngine() *guardrails.GuardrailEngine {
	return guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{}
	}, m.Resolver(), m.PersistenceMgr)
}

func (m *MockAssistantService) Config() *models.Config {
	return &models.Config{}
}

type mockToolProvider struct {
	herder nodeherder.MCPService
}

func (p *mockToolProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	return p.herder.ListTools(ctx)
}

func (p *mockToolProvider) CallTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	return nil, nil
}

func (p *mockToolProvider) GetSystemPrompt() (string, error) {
	return p.herder.GetSystemPrompt()
}

func (p *mockToolProvider) UseNativeTools() bool {
	return true
}

func (m *MockAssistantService) ToolProvider() assistant.ToolProvider {
	return &mockToolProvider{herder: m.Herder}
}

func (m *MockAssistantService) Persistence() *persistence.WorkspaceManager {
	return m.PersistenceMgr
}

func (m *MockAssistantService) ProcessLogger(workspaceID string) logging.Logger {
	return m.LoggerRef
}

func (m *MockAssistantService) RootDir() string {
	return ""
}

func (m *MockAssistantService) WorkspacesDir() string {
	return ""
}

func (m *MockAssistantService) MetadataDir() string {
	return ""
}

func (m *MockAssistantService) Resolver() storage.Resolver {
	return storage.NewPathResolver(m.RootDir(), m.WorkspacesDir(), m.MetadataDir())
}

func (m *MockAssistantService) GuardrailDecisionStore() *assistant.GuardrailDecisionStore {
	return nil
}

func (m *MockAssistantService) Events() assistant.EventPublisher {
	if m.EventBusRef != nil {
		return m.EventBusRef
	}
	return automation.NewEventBus()
}

func (m *MockAssistantService) MemoryStore() *memory.Store { return nil }

func (m *MockAssistantService) Orchestrator() *orchestrator.Orchestrator {
	return nil
}

func (m *MockAssistantService) RecordDir() string { return "" }
func (m *MockAssistantService) RunLoggingEnabled() bool { return true }

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
