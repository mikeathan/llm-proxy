package llm

import (
	"sort"
	"time"

	"llm-proxy/models"
)

// AddModel adds a new model configuration.
func (m *LLMRuntimeManager) AddModel(cfg models.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.models[cfg.Name]; ok {
		return ErrModelExists
	}

	m.models[cfg.Name] = cfg
	return nil
}

// UpdateModel updates an existing model. If the model is active, it will be restarted.
func (m *LLMRuntimeManager) UpdateModel(cfg models.ModelConfig) error {
	m.mu.Lock()

	if _, ok := m.models[cfg.Name]; !ok {
		m.mu.Unlock()
		return ErrUnknownModel
	}

	m.models[cfg.Name] = cfg

	var waiter func()
	if m.activeModel != nil && m.activeModel.cfg.Name == cfg.Name {
		waiter = m.signalStopLocked()
	}
	m.mu.Unlock()

	if waiter != nil {
		waiter()
	}

	return nil
}

// RemoveModel deletes a model. If active, it stops the model.
func (m *LLMRuntimeManager) RemoveModel(name string) error {
	m.mu.Lock()

	cfg, ok := m.models[name]
	if !ok {
		m.mu.Unlock()
		return ErrUnknownModel
	}

	var waiter func()
	if m.activeModel != nil && m.activeModel.cfg.Name == cfg.Name {
		waiter = m.signalStopLocked()
	}

	delete(m.models, name)
	m.mu.Unlock()

	if waiter != nil {
		waiter()
	}
	return nil
}

// ListModels returns all configured models.
func (m *LLMRuntimeManager) ListModels() []models.ModelConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.models))
	for name := range m.models {
		names = append(names, name)
	}
	sort.Strings(names)

	modelsOut := make([]models.ModelConfig, 0, len(names))
	for _, name := range names {
		cfg := m.models[name]
		argsCopy := append([]string(nil), cfg.Args...)

		envCopy := make(map[string]string)
		for k, v := range cfg.Environment {
			envCopy[k] = v
		}

		out := cfg
		out.Args = argsCopy
		out.Environment = envCopy
		modelsOut = append(modelsOut, out)
	}

	return modelsOut
}

// RecordActivity tracks the last usage time for a model.
func (m *LLMRuntimeManager) RecordActivity(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil && m.activeModel.cfg.Name == model {
		m.activeModel.lastUsed = time.Now()
	}
}
