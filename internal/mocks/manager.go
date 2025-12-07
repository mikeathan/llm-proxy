package mocks

import (
	"llm-proxy/internal/proxy"
	"llm-proxy/models"
)

type MockManager struct {
	EnsureModelFunc    func(name string) (proxy.ModelInstance, error)
	RecordActivityFunc func(name string)
	ListModelsFunc     func() []models.ModelConfig
	ActiveInfoFunc     func() *proxy.ActiveModelInfo
	StopActiveFunc     func() error
	ModelHostFunc      func() string
}

func (m *MockManager) EnsureModel(name string) (proxy.ModelInstance, error) {
	return m.EnsureModelFunc(name)
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

func (m *MockManager) ActiveInfo() *proxy.ActiveModelInfo {
	if m.ActiveInfoFunc != nil {
		return m.ActiveInfoFunc()
	}
	return nil
}

func (m *MockManager) StopActive() error {
	if m.StopActiveFunc != nil {
		return m.StopActiveFunc()
	}
	return nil
}

func (m *MockManager) ModelHost() string {
	if m.ModelHostFunc != nil {
		return m.ModelHostFunc()
	}
	return "127.0.0.1"
}
