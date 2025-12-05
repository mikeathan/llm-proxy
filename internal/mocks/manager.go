package mocks


type MockManager struct {
	EnsureModelFunc    func(name string) (int, error)
	RecordActivityFunc func(name string)
}

func (m *MockManager) EnsureModel(name string) (int, error) {
	return m.EnsureModelFunc(name)
}

func (m *MockManager) RecordActivity(name string) {
	if m.RecordActivityFunc != nil {
		m.RecordActivityFunc(name)
	}
}
