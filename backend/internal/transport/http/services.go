package api

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/secrets"
	"llm-proxy/models"
	"time"
)

type RuntimeService interface {
	EnsureModel(context.Context, string) (llm.ModelInstance, error)
	RecordActivity(string)
	ListModels() []models.ModelConfig
	AddModel(models.ModelConfig) error
	UpdateModel(models.ModelConfig) error
	RemoveModel(string) error
	ActiveInfo() *llm.ActiveModelInfo
	ActiveLogs() string
	LastTokensPerSecond() (float64, time.Time)
	StopActive() error
	ClearLogs() error
	ModelHost() string
	SetBinary(string)
	SetModelHost(string)
	ListProviderModels(context.Context, string, string) ([]string, error)
	TestProviderConnection(ctx context.Context, providerName, apiKey, apiKeyName string) error
	SelectModels() (string, string)
}

type AdminService interface {
	ModelDir() string
	SetModelDir(string)
	WorkspacesDir() string
	SetWorkspacesDir(string)
	GPUConfig() models.GPUConfig
	SetGPUConfig(models.GPUConfig)
	CurrentBinary() string
	CurrentIdleTimeout() int
	DefaultArgs() []string
	Environment() map[string]string
	SetEnvironment(map[string]string) error
	Models() []models.ModelConfig
	Providers() map[string]models.ProviderItem
	UpdateConfig(func(*models.Config)) error
	PersistModel(models.ModelConfig) error
	PersistReplaceModel(models.ModelConfig) error
	PersistDeleteModel(string) error
	ResolveModelPath(string, string) string
	RefreshMetricsService()
	MetricsSnapshot() metrics.MetricsSnapshot
	ListMCPServers() []models.MCPServerConfig
	AddMCPServer(models.MCPServerConfig) error
	UpdateMCPServer(models.MCPServerConfig) error
	RemoveMCPServer(string) error
	Config() *models.Config
	SyncGuardrails(models.AgentGuardrailsConfig) error
	ProcessLogger(workspaceID string) logging.Logger
	RootDir() string
	Secrets() secrets.Store
}

type AssistantService interface {
	NodeHerder() nodeherder.MCPService
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	SelectModels() (string, string)

	Engine() assistant.Engine
	ToolProvider() assistant.ToolProvider
	GuardrailEngine() *assistant.GuardrailEngine
	Persistence() *persistence.WorkspaceManager
	Config() *models.Config
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	ProcessLogger(workspaceID string) logging.Logger
	RootDir() string
	Events() *automation.EventBus
}
