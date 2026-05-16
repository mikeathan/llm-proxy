package api

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/models"
	"time"
)

type RuntimeService interface {
	EnsureModel(context.Context, string) (llm.ModelInstance, error)
	RecordActivity(string)
	Sync()
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
	SetModelHost(string)
	ListProviderModels(context.Context, string, string) ([]models.ProviderModelInfo, error)
	TestProviderConnection(ctx context.Context, providerName, apiKey, apiKeyName, baseURL string) error
	SelectModels() (string, string)
}

type AdminService interface {
	// Tier 1: Infrastructure
	GetSystem() models.SystemConfig
	UpdateSystem(func(*models.SystemConfig)) error

	// Tier 2: Registry
	GetRegistry() models.RegistryData
	UpdateRegistry(func(*models.RegistryData)) error

	GetSettings() models.UserSettings
	UpdateSettings(func(*models.UserSettings)) error
	GetGuardrails() models.AgentGuardrailsConfig

	// Tier 3: Secrets
	Secrets() models.SecretsStore

	// Host Machine Isolated Settings
	HostSettings() models.HostSettings
	UpdateHostSettings(models.HostSettings) error

	// Helper accessors for UI / Tools
	WorkspacesDir() string
	GPUConfig() models.GPUConfig
	MetricsSnapshot() metrics.MetricsSnapshot
	ProcessLogger(workspaceID string) logging.Logger
	RootDir() string

	// Model/MCP management
	PersistModel(models.ModelConfig) error
	PersistReplaceModel(models.ModelConfig) error
	PersistDeleteModel(string) error
	DeleteProviderWithCleanup(provider string) error
	ResolveModelPath(string, string) string
	AddMCPServer(models.MCPServerConfig) error
	UpdateMCPServer(models.MCPServerConfig) error
	RemoveMCPServer(string) error
	ListMCPServers() []models.MCPServerConfig
	ListTemplates() ([]models.TemplateMetadata, error)
	GetTemplate(id string) (models.Template, error)
	Models() []models.ModelConfig
	Providers() map[string]models.ProviderItem
	SetGPUConfig(models.GPUConfig)
	SetWorkspacesDir(string)
	Environment() map[string]string
	ApplySystemUpdate(context.Context, models.SystemUpdatePayload) error
	ServiceCredentials() (id, secret string)
	ResetShell(workspaceID string) error
	ListShellSessions() []models.TerminalSessionView
}

type AssistantService interface {
	NodeHerder() nodeherder.MCPService
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	SelectModels() (string, string)

	Engine() assistant.Engine
	ToolProvider() assistant.ToolProvider
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *assistant.GuardrailDecisionStore
	Persistence() *persistence.WorkspaceManager
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	ModelConfig(modelName string) (models.ModelConfig, bool)
	Orchestrator() *orchestrator.Orchestrator
	ProcessLogger(workspaceID string) logging.Logger
	RootDir() string
	Events() *automation.EventBus
}
