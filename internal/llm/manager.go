package llm

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"syscall"
	"time"

	"llm-proxy/internal/testhooks"
	"llm-proxy/models"
)

const (
	defaultLlamaBinary = "llama-server"
	defaultPortStart   = 8081
	defaultReapPeriod  = 10 * time.Second
	shutdownTimeout    = 10 * time.Second
)

var ErrModelStarting = errors.New("model is starting")

type ModelInstance struct {
	Name string
	Host string
	Port int
	Path string
	Args []string
}

type ActiveModelInfo struct {
	Name     string
	Host     string
	Port     int
	Started  time.Time
	LastUsed time.Time
	Ready    bool
}

type RuntimeManager interface {
	EnsureModel(ctx context.Context, name string) (ModelInstance, error)
	RecordActivity(name string)
	ListModels() []models.ModelConfig
	AddModel(models.ModelConfig) error
	UpdateModel(models.ModelConfig) error
	RemoveModel(name string) error
	ActiveInfo() *ActiveModelInfo
	ActiveLogs() string
	LastTokensPerSecond() (float64, time.Time)
	StopActive() error
	ModelHost() string
	SetBinary(path string)
	SetModelHost(host string)
}

type LLMRuntimeManager struct {
	mu          sync.Mutex
	activeModel *runningModel
	models      map[string]models.ModelConfig
	idleTimeout time.Duration
	modelHost   string
	llamaBinary string
	stopCh      chan struct{}
}

var ErrUnknownModel = errors.New("unknown model")
var ErrModelExists = errors.New("model already exists")

func New(modelConfigs []models.ModelConfig, modelHost string, idleTimeout time.Duration) *LLMRuntimeManager {
	return NewWithReapInterval(modelConfigs, modelHost, idleTimeout, defaultReapPeriod)
}

func NewManagerFromConfig(cfg *models.Config) *LLMRuntimeManager {
	modelsOut := make([]models.ModelConfig, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		modelsOut = append(modelsOut, configModelFromConfig(cfg, m))
	}

	manager := New(modelsOut, cfg.Server.ModelHost, time.Duration(cfg.Server.IdleTimeoutSecs)*time.Second)
	if cfg.Server.LlamaServerBinary != "" {
		manager.llamaBinary = cfg.Server.LlamaServerBinary
	}
	return manager
}

func NewWithReapInterval(modelConfigs []models.ModelConfig, modelHost string, idleTimeout, reapInterval time.Duration) *LLMRuntimeManager {
	m := &LLMRuntimeManager{
		models:      make(map[string]models.ModelConfig),
		idleTimeout: idleTimeout,
		modelHost:   modelHost,
		llamaBinary: defaultLlamaBinary,
		stopCh:      make(chan struct{}),
	}

	for _, mc := range modelConfigs {
		normalized := normalizeModelConfig("", mc)
		m.models[normalized.Name] = normalized
	}

	go m.reapIdleModels(reapInterval)

	return m
}
func (m *LLMRuntimeManager) EnsureModel(ctx context.Context, name string) (ModelInstance, error) {
	m.mu.Lock()
	// Loop to handle "unlock-wait-lock" retry pattern
	for {
		cfg, ok := m.models[name]
		if !ok {
			m.mu.Unlock()
			return ModelInstance{}, ErrUnknownModel
		}

		cfg = m.syncPortWithActiveLocked(cfg)

		if inst, ok := m.readyInstanceLocked(name, cfg); ok {
			m.mu.Unlock()
			return inst, nil
		}

		if m.activeModel != nil && m.activeModel.cfg.Name == name {
			m.mu.Unlock()
			return ModelInstance{}, ErrModelStarting
		}

		// Need to stop another active model?
		activePort := m.activePortLocked()
		waiter := m.signalStopLocked()
		if waiter != nil {
			m.mu.Unlock()
			waiter() // Wait for process to exit without holding the lock
			m.mu.Lock()
			continue // Retry checking state
		}

		// No active model, start the new one
		cfg.Port = m.defaultPortLocked(cfg, activePort)
		m.models[name] = cfg

		err := m.startModelLocked(ctx, cfg)
		m.mu.Unlock()
		if err != nil {
			return ModelInstance{}, err
		}

		return ModelInstance{}, ErrModelStarting
	}
}

func (m *LLMRuntimeManager) RecordActivity(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil && m.activeModel.cfg.Name == model {
		m.activeModel.lastUsed = time.Now()
	}
}

func (m *LLMRuntimeManager) StopActive() error {
	m.mu.Lock()
	waiter := m.signalStopLocked()
	m.mu.Unlock()
	if waiter != nil {
		waiter()
	}
	return nil
}

func (m *LLMRuntimeManager) AddModel(cfg models.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.models[cfg.Name]; ok {
		return ErrModelExists
	}

	m.models[cfg.Name] = cfg
	return nil
}

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
		modelsOut = append(modelsOut, models.ModelConfig{
			Name:     cfg.Name,
			Filename: cfg.Filename,
			Path:     cfg.Path,
			Args:     argsCopy,
			Port:     cfg.Port,
		})
	}

	return modelsOut
}

func (m *LLMRuntimeManager) ActiveInfo() *ActiveModelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel == nil {
		return nil
	}

	cfg := m.activeModel.cfg

	return &ActiveModelInfo{
		Name:     cfg.Name,
		Host:     m.modelHost,
		Port:     cfg.Port,
		Started:  m.activeModel.started,
		LastUsed: m.activeModel.lastUsed,
		Ready:    testhooks.PortReady(cfg.Port),
	}
}

// ActiveLogs returns the buffered stdout/stderr for the active model.
func (m *LLMRuntimeManager) ActiveLogs() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel == nil || m.activeModel.logs == nil {
		return ""
	}
	return m.activeModel.logs.String()
}

// LastTokensPerSecond returns the most recent throughput reported by llama-server output.
func (m *LLMRuntimeManager) LastTokensPerSecond() (float64, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel != nil && m.activeModel.throughput != nil {
		return m.activeModel.throughput.LastTokensPerSecond()
	}
	return 0, time.Time{}
}

func (m *LLMRuntimeManager) signalStopLocked() func() {
	if m.activeModel == nil {
		return nil
	}

	cmd := m.activeModel.cmd
	if m.activeModel.cancel != nil {
		m.activeModel.cancel()
	}

	// Try graceful stop
	if cmd.Process != nil {
		if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	m.activeModel = nil

	return func() {
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			if cmd.Process != nil {
				if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					_ = cmd.Process.Kill()
				}
			}
		}
	}
}

func (m *LLMRuntimeManager) ActiveModel() *runningModel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeModel
}

func (m *LLMRuntimeManager) ModelHost() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.modelHost
}

func (m *LLMRuntimeManager) SetBinary(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if path != "" {
		m.llamaBinary = path
	}
}

func (m *LLMRuntimeManager) SetModelHost(host string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if host != "" {
		m.modelHost = host
	}
}

func (m *LLMRuntimeManager) defaultPortLocked(cfg models.ModelConfig, activePort int) int {
	if cfg.Port != 0 {
		return cfg.Port
	}
	if activePort != 0 {
		return activePort
	}
	port := defaultPortStart
	for _, mc := range m.models {
		if mc.Port >= port {
			port = mc.Port + 1
		}
	}
	return port
}

func (m *LLMRuntimeManager) reapIdleModels(reapInterval time.Duration) {
	t := time.NewTicker(reapInterval)
	defer t.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-t.C:
			m.mu.Lock()

			if m.activeModel != nil {
				if time.Since(m.activeModel.lastUsed) > m.idleTimeout {
					log.Printf("Idle timeout on model %s → stopping", m.activeModel.cfg.Name)
					waiter := m.signalStopLocked()
					m.mu.Unlock()
					if waiter != nil {
						waiter()
					}
					continue
				}
			}

			m.mu.Unlock()
		}
	}
}

// Shutdown stops the background reaper and any active model.
func (m *LLMRuntimeManager) Shutdown() {
	close(m.stopCh)
	m.mu.Lock()
	waiter := m.signalStopLocked()
	m.mu.Unlock()
	if waiter != nil {
		waiter()
	}
}
