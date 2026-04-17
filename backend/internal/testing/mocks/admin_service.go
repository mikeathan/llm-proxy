package mocks

import (
	"context"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
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
	GetSystemFunc             func() models.SystemConfig
	UpdateSystemFunc          func(func(*models.SystemConfig)) error
	GetRegistryFunc           func() models.RegistryData
	UpdateRegistryFunc        func(func(*models.RegistryData)) error
	PersistModelFunc          func(models.ModelConfig) error
	PersistReplaceModelFunc   func(models.ModelConfig) error
	PersistDeleteModelFunc    func(string) error
	ResolveModelPathFunc      func(string, string) string
	RefreshMetricsServiceFunc func()
	MetricsSnapshotFunc       func() metrics.MetricsSnapshot
	ListMCPServersFunc        func() []models.MCPServerConfig
	AddMCPServerFunc          func(models.MCPServerConfig) error
	UpdateMCPServerFunc       func(models.MCPServerConfig) error
	RemoveMCPServerFunc       func(string) error
	EnvironmentFunc           func() map[string]string
	SetEnvironmentFunc        func(map[string]string) error
	ModelsFunc                func() []models.ModelConfig
	ProvidersFunc             func() map[string]models.ProviderItem
	WorkspacesDirFunc         func() string
	SetWorkspacesDirFunc      func(string)
	SyncGuardrailsFunc        func(models.AgentGuardrailsConfig) error
	ProcessLoggerFunc         func(string) logging.Logger
	RootDirFunc               func() string
	SecretsFunc               func() models.SecretsStore
	UpdateSettingsFunc        func(context.Context, models.SystemUpdatePayload) error
	ServiceCredentialsFunc    func() (string, string)
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

func (m *MockAdminService) WorkspacesDir() string {
	if m.WorkspacesDirFunc != nil {
		return m.WorkspacesDirFunc()
	}
	return ""
}

func (m *MockAdminService) SetWorkspacesDir(dir string) {
	if m.SetWorkspacesDirFunc != nil {
		m.SetWorkspacesDirFunc(dir)
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

func (m *MockAdminService) GetSystem() models.SystemConfig {
	if m.GetSystemFunc != nil {
		return m.GetSystemFunc()
	}
	return models.SystemConfig{}
}

func (m *MockAdminService) UpdateSystem(fn func(*models.SystemConfig)) error {
	if m.UpdateSystemFunc != nil {
		return m.UpdateSystemFunc(fn)
	}
	return nil
}

func (m *MockAdminService) GetRegistry() models.RegistryData {
	if m.GetRegistryFunc != nil {
		return m.GetRegistryFunc()
	}
	return models.RegistryData{}
}

func (m *MockAdminService) UpdateRegistry(fn func(*models.RegistryData)) error {
	if m.UpdateRegistryFunc != nil {
		return m.UpdateRegistryFunc(fn)
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

func (m *MockAdminService) MetricsSnapshot() metrics.MetricsSnapshot {
	if m.MetricsSnapshotFunc != nil {
		return m.MetricsSnapshotFunc()
	}
	return metrics.MetricsSnapshot{}
}

func (m *MockAdminService) ListMCPServers() []models.MCPServerConfig {
	if m.ListMCPServersFunc != nil {
		return m.ListMCPServersFunc()
	}
	return nil
}

func (m *MockAdminService) AddMCPServer(cfg models.MCPServerConfig) error {
	if m.AddMCPServerFunc != nil {
		return m.AddMCPServerFunc(cfg)
	}
	return nil
}

func (m *MockAdminService) UpdateMCPServer(cfg models.MCPServerConfig) error {
	if m.UpdateMCPServerFunc != nil {
		return m.UpdateMCPServerFunc(cfg)
	}
	return nil
}

func (m *MockAdminService) RemoveMCPServer(name string) error {
	if m.RemoveMCPServerFunc != nil {
		return m.RemoveMCPServerFunc(name)
	}
	return nil
}

func (m *MockAdminService) Environment() map[string]string {
	if m.EnvironmentFunc != nil {
		return m.EnvironmentFunc()
	}
	return map[string]string{}
}

func (m *MockAdminService) SetEnvironment(env map[string]string) error {
	if m.SetEnvironmentFunc != nil {
		return m.SetEnvironmentFunc(env)
	}
	return nil
}

func (m *MockAdminService) Models() []models.ModelConfig {
	if m.ModelsFunc != nil {
		return m.ModelsFunc()
	}
	return nil
}
func (m *MockAdminService) Providers() map[string]models.ProviderItem {
	if m.ProvidersFunc != nil {
		return m.ProvidersFunc()
	}
	return nil
}

func (m *MockAdminService) SyncGuardrails(cfg models.AgentGuardrailsConfig) error {
	if m.SyncGuardrailsFunc != nil {
		return m.SyncGuardrailsFunc(cfg)
	}
	return nil
}

func (m *MockAdminService) ProcessLogger(workspaceID string) logging.Logger {
	if m.ProcessLoggerFunc != nil {
		return m.ProcessLoggerFunc(workspaceID)
	}
	return nil
}

func (m *MockAdminService) RootDir() string {
	if m.RootDirFunc != nil {
		return m.RootDirFunc()
	}
	return ""
}

func (m *MockAdminService) Secrets() models.SecretsStore {
	if m.SecretsFunc != nil {
		return m.SecretsFunc()
	}
	return nil
}

func (m *MockAdminService) UpdateSettings(ctx context.Context, req models.SystemUpdatePayload) error {
	if m.UpdateSettingsFunc != nil {
		return m.UpdateSettingsFunc(ctx, req)
	}
	return nil
}

func (m *MockAdminService) ServiceCredentials() (string, string) {
	if m.ServiceCredentialsFunc != nil {
		return m.ServiceCredentialsFunc()
	}
	return "", ""
}

