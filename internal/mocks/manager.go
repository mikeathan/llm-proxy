package mocks

import "llm-proxy/internal/proxy"

type MockManager struct {
	EnsureModelFunc    func(name string) (proxy.ModelInstance, error)
	RecordActivityFunc func(name string)
}

func (m *MockManager) EnsureModel(name string) (proxy.ModelInstance, error) {
	return m.EnsureModelFunc(name)
}

func (m *MockManager) RecordActivity(name string) {
	if m.RecordActivityFunc != nil {
		m.RecordActivityFunc(name)
	}
}
