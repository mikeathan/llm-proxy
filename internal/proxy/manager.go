package proxy

import (
	"errors"
	"fmt"
	"llm-proxy/models"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var ErrModelStarting = errors.New("model is starting")

type ModelConfig struct {
	Name string
	Path string
	Args []string
	Port int
}

type LLMProxyManager interface {
	EnsureModel(name string) (int, error)
	RecordActivity(name string)
}

type LLMManager struct {
	mu          sync.Mutex
	activeModel *runningModel
	models      map[string]ModelConfig
	idleTimeout time.Duration
}

type runningModel struct {
	cfg      ModelConfig
	cmd      *exec.Cmd
	started  time.Time
	lastUsed time.Time
}

func (rm *runningModel) LastUsed() time.Time {
	return rm.lastUsed
}

func (rm *runningModel) Started() time.Time {
	return rm.started
}

func (rm *runningModel) Cfg() ModelConfig {
	return rm.cfg
}

var ErrUnknownModel = errors.New("unknown model")

func New(modelConfigs []ModelConfig, idleTimeout time.Duration) *LLMManager {
	return NewWithReapInterval(modelConfigs, idleTimeout, 10*time.Second)
}

func NewManagerFromConfig(cfg *models.Config) *LLMManager {
	models := make([]ModelConfig, len(cfg.Models))

	for i, m := range cfg.Models {
		args := append(cfg.Server.DefaultArgs, m.Args...)

		models[i] = ModelConfig{
			Name: m.Name,
			Path: m.Path,
			Args: args,
			Port: m.Port,
		}
	}

	return New(models, time.Duration(cfg.Server.IdleTimeoutSecs)*time.Second)
}
func NewWithReapInterval(modelConfigs []ModelConfig, idleTimeout, reapInterval time.Duration) *LLMManager {
	m := &LLMManager{
		models:      make(map[string]ModelConfig),
		idleTimeout: idleTimeout,
	}

	for _, mc := range modelConfigs {
		m.models[mc.Name] = mc
	}

	go m.reapIdleModels(reapInterval)

	return m
}

func (m *LLMManager) EnsureModel(name string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.models[name]
	if !ok {
		return 0, ErrUnknownModel
	}

	// Model already running
	if m.activeModel != nil && m.activeModel.cfg.Name == name {
		m.activeModel.lastUsed = time.Now()
		return cfg.Port, nil
	}

	// If model is starting but not yet ready
	if m.activeModel != nil && m.activeModel.cfg.Name == name && !portReadyFunc(cfg.Port) {
		return 0, ErrModelStarting
	}

	// Stop old model
	if m.activeModel != nil {
		_ = m.stopLocked()
	}

	// Start new model process
	cmd := execCommand(
		"llama-server",
		append([]string{"-m", cfg.Path, "--port", fmt.Sprint(cfg.Port)}, cfg.Args...)...,
	)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("model start failed: %w", err)
	}

	m.activeModel = &runningModel{
		cfg:      cfg,
		cmd:      cmd,
		started:  time.Now(),
		lastUsed: time.Now(),
	}

	// Immediately return "starting"
	return 0, ErrModelStarting
}

func (m *LLMManager) RecordActivity(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil && m.activeModel.cfg.Name == model {
		m.activeModel.lastUsed = time.Now()
	}
}

func (m *LLMManager) stopLocked() error {
	if m.activeModel == nil {
		return nil
	}

	cmd := m.activeModel.cmd

	// Try graceful stop
	_ = cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
	}

	m.activeModel = nil
	return nil
}

func (m *LLMManager) ActiveModel() *runningModel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeModel
}

func (m *LLMManager) reapIdleModels(reapInterval time.Duration) {
	t := time.NewTicker(reapInterval)
	defer t.Stop()

	for range t.C {
		m.mu.Lock()

		if m.activeModel != nil {
			if time.Since(m.activeModel.lastUsed) > m.idleTimeout {
				log.Printf("Idle timeout on model %s → stopping", m.activeModel.cfg.Name)
				_ = m.stopLocked()
			}
		}

		m.mu.Unlock()
	}
}
