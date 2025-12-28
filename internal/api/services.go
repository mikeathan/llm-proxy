package api

import (
	"llm-proxy/internal/llm"
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
