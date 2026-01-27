package mocks

import (
	"llm-proxy/internal/system_metrics"
	"llm-proxy/models"
)

type MockAdminService struct {
	ModelDirFunc              func() string
	SetModelDirFunc           func(string)
	GPUConfigFunc             func() models.GPUConfig
	SetGPUConfigFunc          func(models.GPUConfig)
	CurrentBinaryFunc         func() string
	CurrentIdleTimeoutFunc    func() int
	DefaultArgsFunc           func() []string
	UpdateConfigFunc          func(func(*models.Config)) error
	PersistModelFunc          func(models.ModelConfig) error
	PersistReplaceModelFunc   func(models.ModelConfig) error
	PersistDeleteModelFunc    func(string) error
	ResolveModelPathFunc      func(string, string) string
	RefreshMetricsServiceFunc func()
	MetricsSnapshotFunc       func() system_metrics.MetricsSnapshot
}

func (m *MockAdminService) ModelDir() string {
	if m.ModelDirFunc != nil {
		return m.ModelDirFunc()
	}
	return ""
}

func (m *MockAdminService) SetModelDir(dir string) {
	if m.SetModelDirFunc != nil {
		m.SetModelDirFunc(dir)
	}
}

func (m *MockAdminService) GPUConfig() models.GPUConfig {
	if m.GPUConfigFunc != nil {
		return m.GPUConfigFunc()
	}
	return models.GPUConfig{}
}

func (m *MockAdminService) SetGPUConfig(cfg models.GPUConfig) {
	if m.SetGPUConfigFunc != nil {
		m.SetGPUConfigFunc(cfg)
	}
}

func (m *MockAdminService) CurrentBinary() string {
	if m.CurrentBinaryFunc != nil {
		return m.CurrentBinaryFunc()
	}
	return ""
}

func (m *MockAdminService) CurrentIdleTimeout() int {
	if m.CurrentIdleTimeoutFunc != nil {
		return m.CurrentIdleTimeoutFunc()
	}
	return 0
}

func (m *MockAdminService) DefaultArgs() []string {
	if m.DefaultArgsFunc != nil {
		return m.DefaultArgsFunc()
	}
	return nil
}

func (m *MockAdminService) UpdateConfig(fn func(*models.Config)) error {
	if m.UpdateConfigFunc != nil {
		return m.UpdateConfigFunc(fn)
	}
	return nil
}

func (m *MockAdminService) PersistModel(cfg models.ModelConfig) error {
	if m.PersistModelFunc != nil {
		return m.PersistModelFunc(cfg)
	}
	return nil
}

func (m *MockAdminService) PersistReplaceModel(cfg models.ModelConfig) error {
	if m.PersistReplaceModelFunc != nil {
		return m.PersistReplaceModelFunc(cfg)
	}
	return nil
}

func (m *MockAdminService) PersistDeleteModel(name string) error {
	if m.PersistDeleteModelFunc != nil {
		return m.PersistDeleteModelFunc(name)
	}
	return nil
}

func (m *MockAdminService) ResolveModelPath(filename string, path string) string {
	if m.ResolveModelPathFunc != nil {
		return m.ResolveModelPathFunc(filename, path)
	}
	return ""
}

func (m *MockAdminService) RefreshMetricsService() {
	if m.RefreshMetricsServiceFunc != nil {
		m.RefreshMetricsServiceFunc()
	}
}

func (m *MockAdminService) MetricsSnapshot() system_metrics.MetricsSnapshot {
	if m.MetricsSnapshotFunc != nil {
		return m.MetricsSnapshotFunc()
	}
	return system_metrics.MetricsSnapshot{}
}
