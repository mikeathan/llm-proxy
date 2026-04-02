package api

import (
	"context"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/metrics"
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
}

type AssistantService interface {
	NodeHerder() nodeherder.MCPService
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	DefaultModel() (string, error)

	Engine() assistant.Engine
}
