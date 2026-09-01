package mocks

import (
	"context"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/models"
	"time"
)

type MockManager struct {
	EnsureModelFunc             func(ctx context.Context, name string) (llm.ModelInstance, error)
	GetInstanceFunc             func(ctx context.Context, name string) (llm.ModelInstance, error)
	RecordActivityFunc          func(name string)
	ListModelsFunc              func() []models.ModelConfig
	AddModelFunc                func(models.ModelConfig) error
	UpdateModelFunc             func(models.ModelConfig) error
	RemoveModelFunc             func(string) error
	ActiveInfoFunc              func() *llm.ActiveModelInfo
	ActiveLogsFunc              func() string
	LastTokensPerSecondFunc     func() (float64, time.Time)
	LastModelErrorFunc          func() string
	StopActiveFunc              func() error
	ClearLogsFunc               func() error
	ModelHostFunc               func() string
	SetBinaryFunc               func(string)
	SetModelHostFunc            func(string)
	ListProviderModelsFunc      func(ctx context.Context, provider, apiKeyName string) ([]models.ProviderModelInfo, error)
	TestProviderConnectionFunc  func(ctx context.Context, provider, apiKey, apiKeyName, baseURL string) error
	ProbeModelAvailabilityFunc  func(ctx context.Context, cfg models.ModelConfig) error
	EffectiveToolCallFormatFunc func(ctx context.Context, name string) string
	SelectModelsFunc            func() (string, string)
	SetSecretsFunc              func(models.SecretsStore)
	ShutdownFunc                func()
	RegistrarFunc               func() *providers.ProviderRegistrar
	SyncFunc                    func()
	ClassifyModelFunc           func(models.ModelConfig) models.WorkloadClass
}

func (m *MockManager) Registrar() *providers.ProviderRegistrar {
	if m.RegistrarFunc != nil {
		return m.RegistrarFunc()
	}
	return nil
}

func NewMockManager() *MockManager {
	return (&MockManager{}).WithDefaultRegistrar()
}

func (m *MockManager) WithDefaultRegistrar() *MockManager {
	m.RegistrarFunc = func() *providers.ProviderRegistrar {
		return providers.NewProviderRegistrar(providers.GetRegistry(), nil, "http://localhost")
	}
	return m
}

func (m *MockManager) Shutdown() {
	if m.ShutdownFunc != nil {
		m.ShutdownFunc()
	}
}

func (m *MockManager) SelectModels() (string, string) {
	if m.SelectModelsFunc != nil {
		return m.SelectModelsFunc()
	}
	return "", ""
}

func (m *MockManager) TestProviderConnection(ctx context.Context, provider, apiKey, apiKeyName, baseURL string) error {
	if m.TestProviderConnectionFunc != nil {
		return m.TestProviderConnectionFunc(ctx, provider, apiKey, apiKeyName, baseURL)
	}
	return nil
}

func (m *MockManager) ListProviderModels(ctx context.Context, provider, apiKeyName string) ([]models.ProviderModelInfo, error) {
	if m.ListProviderModelsFunc != nil {
		return m.ListProviderModelsFunc(ctx, provider, apiKeyName)
	}
	return nil, nil
}

func (m *MockManager) ProbeModelAvailability(ctx context.Context, cfg models.ModelConfig) error {
	if m.ProbeModelAvailabilityFunc != nil {
		return m.ProbeModelAvailabilityFunc(ctx, cfg)
	}
	return nil
}

// EffectiveToolCallFormat defaults to the mock's explicit override function;
// tests that need a real resolution can point EffectiveToolCallFormatFunc at
// a stub (e.g. return "native").
func (m *MockManager) EffectiveToolCallFormat(ctx context.Context, name string) string {
	if m.EffectiveToolCallFormatFunc != nil {
		return m.EffectiveToolCallFormatFunc(ctx, name)
	}
	return ""
}

// ClassifyModel defaults to the pure classifier (provider label, GGUF
// artifact, effective endpoint host on ProviderConfig.BaseURL) so handler
// tests keep working without a registrar; tests that exercise credential
// hydration override ClassifyModelFunc.
func (m *MockManager) ClassifyModel(cfg models.ModelConfig) models.WorkloadClass {
	if m.ClassifyModelFunc != nil {
		return m.ClassifyModelFunc(cfg)
	}
	return models.NewWorkloadClassifier("", nil).Classify(cfg)
}

func (m *MockManager) EnsureModel(ctx context.Context, name string) (llm.ModelInstance, error) {
	return m.EnsureModelFunc(ctx, name)
}

func (m *MockManager) GetInstance(ctx context.Context, name string) (llm.ModelInstance, error) {
	if m.GetInstanceFunc != nil {
		return m.GetInstanceFunc(ctx, name)
	}
	// Default to EnsureModel behavior in mocks unless specified
	return m.EnsureModelFunc(ctx, name)
}

func (m *MockManager) RecordActivity(name string) {
	if m.RecordActivityFunc != nil {
		m.RecordActivityFunc(name)
	}
}

func (m *MockManager) ListModels() []models.ModelConfig {
	if m.ListModelsFunc != nil {
		return m.ListModelsFunc()
	}
	return nil
}

func (m *MockManager) AddModel(cfg models.ModelConfig) error {
	if m.AddModelFunc != nil {
		return m.AddModelFunc(cfg)
	}
	return nil
}

func (m *MockManager) UpdateModel(cfg models.ModelConfig) error {
	if m.UpdateModelFunc != nil {
		return m.UpdateModelFunc(cfg)
	}
	return nil
}

func (m *MockManager) RemoveModel(name string) error {
	if m.RemoveModelFunc != nil {
		return m.RemoveModelFunc(name)
	}
	return nil
}

func (m *MockManager) ActiveInfo() *llm.ActiveModelInfo {
	if m.ActiveInfoFunc != nil {
		return m.ActiveInfoFunc()
	}
	return nil
}

func (m *MockManager) ActiveLogs() string {
	if m.ActiveLogsFunc != nil {
		return m.ActiveLogsFunc()
	}
	return ""
}

func (m *MockManager) LastTokensPerSecond() (float64, time.Time) {
	if m.LastTokensPerSecondFunc != nil {
		return m.LastTokensPerSecondFunc()
	}
	return 0, time.Time{}
}

func (m *MockManager) LastModelError() string {
	if m.LastModelErrorFunc != nil {
		return m.LastModelErrorFunc()
	}
	return ""
}

func (m *MockManager) StopActive() error {
	if m.StopActiveFunc != nil {
		return m.StopActiveFunc()
	}
	return nil
}

func (m *MockManager) ClearLogs() error {
	if m.ClearLogsFunc != nil {
		return m.ClearLogsFunc()
	}
	return nil
}

func (m *MockManager) ModelHost() string {
	if m.ModelHostFunc != nil {
		return m.ModelHostFunc()
	}
	return "127.0.0.1"
}

func (m *MockManager) SetBinary(path string) {
	if m.SetBinaryFunc != nil {
		m.SetBinaryFunc(path)
	}
}

func (m *MockManager) SetSecrets(s models.SecretsStore) {
	if m.SetSecretsFunc != nil {
		m.SetSecretsFunc(s)
	}
}

func (m *MockManager) SetModelHost(host string) {
	if m.SetModelHostFunc != nil {
		m.SetModelHostFunc(host)
	}
}

func (m *MockManager) Sync() {
	if m.SyncFunc != nil {
		// Mock sync behavior doesn't usually need the providers, but we pass them if needed
		m.SyncFunc()
	}
}

func (m *MockManager) ApplyModelOverrides(overrides map[string]models.ModelOverride) {
	// no-op for mock
}
