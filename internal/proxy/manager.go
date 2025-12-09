package proxy

import (
	"errors"
	"fmt"
	"io"
	"llm-proxy/models"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
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

type LLMProxyManager interface {
	EnsureModel(name string) (ModelInstance, error)
	RecordActivity(name string)
	ListModels() []models.ModelConfig
	AddModel(models.ModelConfig) error
	UpdateModel(models.ModelConfig) error
	RemoveModel(name string) error
	ActiveInfo() *ActiveModelInfo
	ActiveLogs() string
	StopActive() error
	ModelHost() string
	SetBinary(path string)
}

type LLMManager struct {
	mu          sync.Mutex
	activeModel *runningModel
	models      map[string]models.ModelConfig
	idleTimeout time.Duration
	modelHost   string
	llamaBinary string
}

type runningModel struct {
	cfg      models.ModelConfig
	cmd      *exec.Cmd
	started  time.Time
	lastUsed time.Time
	logs     *logBuffer
}

func (rm *runningModel) LastUsed() time.Time {
	return rm.lastUsed
}

func (rm *runningModel) Started() time.Time {
	return rm.started
}

func (rm *runningModel) Cfg() models.ModelConfig {
	return rm.cfg
}

var ErrUnknownModel = errors.New("unknown model")
var ErrModelExists = errors.New("model already exists")

func resolveModelFile(baseDir string, m models.ModelConfig) string {
	if m.Path != "" && filepath.IsAbs(m.Path) {
		return m.Path
	}
	fname := m.Filename
	if fname == "" && m.Path != "" {
		fname = filepath.Base(m.Path)
	}
	if fname == "" {
		return ""
	}
	if filepath.IsAbs(fname) {
		return fname
	}
	if baseDir != "" {
		return filepath.Join(baseDir, fname)
	}
	return fname
}

func New(modelConfigs []models.ModelConfig, modelHost string, idleTimeout time.Duration) *LLMManager {
	return NewWithReapInterval(modelConfigs, modelHost, idleTimeout, 10*time.Second)
}

func NewManagerFromConfig(cfg *models.Config) *LLMManager {
	mc := make([]models.ModelConfig, len(cfg.Models))

	for i, m := range cfg.Models {
		args := append(cfg.Server.DefaultArgs, m.Args...)
		fullPath := resolveModelFile(cfg.ModelDir, m)

		mc[i] = models.ModelConfig{
			Name:     m.Name,
			Filename: m.Filename,
			Path:     fullPath,
			Args:     args,
			Port:     m.Port,
		}

	}

	m := New(mc, cfg.Server.ModelHost, time.Duration(cfg.Server.IdleTimeoutSecs)*time.Second)
	if cfg.Server.LlamaServerBinary != "" {
		m.llamaBinary = cfg.Server.LlamaServerBinary
	}
	return m
}

func NewWithReapInterval(modelConfigs []models.ModelConfig, modelHost string, idleTimeout, reapInterval time.Duration) *LLMManager {
	m := &LLMManager{
		models:      make(map[string]models.ModelConfig),
		idleTimeout: idleTimeout,
		modelHost:   modelHost,
		llamaBinary: "llama-server",
	}

	for _, mc := range modelConfigs {
		if mc.Path == "" {
			mc.Path = resolveModelFile("", mc)
		}
		if mc.Filename == "" && mc.Path != "" {
			mc.Filename = filepath.Base(mc.Path)
		}
		m.models[mc.Name] = mc
	}

	go m.reapIdleModels(reapInterval)

	return m
}
func (m *LLMManager) EnsureModel(name string) (ModelInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.models[name]
	if !ok {
		return ModelInstance{}, ErrUnknownModel
	}

	// Already running AND ready
	if m.activeModel != nil && m.activeModel.cfg.Name == name && cfg.Port == 0 {
		cfg.Port = m.activeModel.cfg.Port
		m.models[name] = cfg
	}
	if m.activeModel != nil && m.activeModel.cfg.Name == name && portReadyFunc(cfg.Port) {
		m.activeModel.lastUsed = time.Now()
		return ModelInstance{
			Name: cfg.Name,
			Host: m.modelHost,
			Port: cfg.Port,
			Path: cfg.Path,
			Args: cfg.Args,
		}, nil
	}

	// Already running BUT still starting
	if m.activeModel != nil && m.activeModel.cfg.Name == name && !portReadyFunc(cfg.Port) {
		return ModelInstance{}, ErrModelStarting
	}

	// Stop previously running model
	activePort := 0
	if m.activeModel != nil {
		activePort = m.activeModel.cfg.Port
	}
	if m.activeModel != nil {
		_ = m.stopLocked()
	}

	port := m.defaultPortLocked(cfg, activePort)
	cfg.Port = port
	m.models[name] = cfg

	// Start new model process
	logBuf := newLogBuffer(64 * 1024)
	cmd := execCommand(
		m.llamaBinary,
		append([]string{"-m", cfg.Path, "--port", fmt.Sprint(cfg.Port)}, cfg.Args...)...,
	)
	cmd.Stdout = io.MultiWriter(logBuf, os.Stdout)
	cmd.Stderr = io.MultiWriter(logBuf, os.Stdout)

	if err := cmd.Start(); err != nil {
		return ModelInstance{}, fmt.Errorf("model start failed: %w", err)
	}

	m.activeModel = &runningModel{
		cfg:      cfg,
		cmd:      cmd,
		started:  time.Now(),
		lastUsed: time.Now(),
		logs:     logBuf,
	}

	// Model is starting, not ready
	return ModelInstance{}, ErrModelStarting
}

func (m *LLMManager) RecordActivity(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil && m.activeModel.cfg.Name == model {
		m.activeModel.lastUsed = time.Now()
	}
}

func (m *LLMManager) StopActive() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *LLMManager) AddModel(cfg models.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.models[cfg.Name]; ok {
		return ErrModelExists
	}

	m.models[cfg.Name] = cfg
	return nil
}

func (m *LLMManager) UpdateModel(cfg models.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.models[cfg.Name]; !ok {
		return ErrUnknownModel
	}

	m.models[cfg.Name] = cfg

	if m.activeModel != nil && m.activeModel.cfg.Name == cfg.Name {
		_ = m.stopLocked()
	}

	return nil
}

func (m *LLMManager) RemoveModel(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.models[name]
	if !ok {
		return ErrUnknownModel
	}

	if m.activeModel != nil && m.activeModel.cfg.Name == cfg.Name {
		_ = m.stopLocked()
	}

	delete(m.models, name)
	return nil
}

func (m *LLMManager) ListModels() []models.ModelConfig {
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

func (m *LLMManager) ActiveInfo() *ActiveModelInfo {
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
		Ready:    portReadyFunc(cfg.Port),
	}
}

// ActiveLogs returns the buffered stdout/stderr for the active model.
func (m *LLMManager) ActiveLogs() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel == nil || m.activeModel.logs == nil {
		return ""
	}
	return m.activeModel.logs.String()
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

func (m *LLMManager) ModelHost() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.modelHost
}

func (m *LLMManager) SetBinary(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if path != "" {
		m.llamaBinary = path
	}
}

func (m *LLMManager) defaultPortLocked(cfg models.ModelConfig, activePort int) int {
	if cfg.Port != 0 {
		return cfg.Port
	}
	if activePort != 0 {
		return activePort
	}
	port := 8081
	for _, mc := range m.models {
		if mc.Port >= port {
			port = mc.Port + 1
		}
	}
	return port
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
