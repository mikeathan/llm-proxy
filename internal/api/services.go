package api

import (
	"llm-proxy/internal/assistant"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/internal/system_metrics"
	"llm-proxy/models"
	"time"
)

type RuntimeService interface {
	EnsureModel(string) (llm.ModelInstance, error)
	RecordActivity(string)
	ListModels() []models.ModelConfig
	AddModel(models.ModelConfig) error
	UpdateModel(models.ModelConfig) error
	RemoveModel(string) error
	ActiveInfo() *llm.ActiveModelInfo
	ActiveLogs() string
	LastTokensPerSecond() (float64, time.Time)
	StopActive() error
	ModelHost() string
	SetBinary(string)
	SetModelHost(string)
}

type AdminService interface {
	ModelDir() string
	SetModelDir(string)
	GPUConfig() models.GPUConfig
	SetGPUConfig(models.GPUConfig)
	CurrentBinary() string
	CurrentIdleTimeout() int
	DefaultArgs() []string
	UpdateConfig(func(*models.Config)) error
	PersistModel(models.ModelConfig) error
	PersistReplaceModel(models.ModelConfig) error
	PersistDeleteModel(string) error
	ResolveModelPath(string, string) string
	RefreshMetricsService()
	MetricsSnapshot() system_metrics.MetricsSnapshot
}

type AssistantService interface {
	NodeHerder() nodeherder.NodeHerderService
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	DefaultModel() (string, error)

	Engine() assistant.Engine
}
