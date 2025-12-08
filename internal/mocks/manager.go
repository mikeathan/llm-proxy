package mocks

import (
	"llm-proxy/internal/proxy"
	"llm-proxy/models"
)

type MockManager struct {
	EnsureModelFunc    func(name string) (proxy.ModelInstance, error)
	RecordActivityFunc func(name string)
	ListModelsFunc     func() []models.ModelConfig
	AddModelFunc       func(models.ModelConfig) error
	UpdateModelFunc    func(models.ModelConfig) error
	RemoveModelFunc    func(string) error
	ActiveInfoFunc     func() *proxy.ActiveModelInfo
	StopActiveFunc     func() error
	ModelHostFunc      func() string
	SetBinaryFunc      func(string)
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

func (m *MockManager) SetBinary(path string) {
	if m.SetBinaryFunc != nil {
		m.SetBinaryFunc(path)
	}
}
