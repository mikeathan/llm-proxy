package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/testing/utils"
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
	Name    string
	Host    string
	Port    int
	Path    string
	Args    []string
	URL     string
	Headers http.Header
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
	ClearLogs() error
	ModelHost() string
	SetBinary(path string)
	SetModelHost(host string)
	ListProviderModels(ctx context.Context, provider string) ([]string, error)
	TestProviderConnection(ctx context.Context, provider, apiKey string) error
}

type LLMRuntimeManager struct {
	mu                sync.Mutex
	activeModel       *runningModel
	activeProvider    Provider
	activeCloudConfig *models.ModelConfig
	models            map[string]models.ModelConfig
	providers         map[string]models.ProviderItem
	idleTimeout       time.Duration
	modelHost         string
	llamaBinary       string
	stopCh            chan struct{}
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
	manager.providers = cfg.Providers
	
	if local, ok := cfg.Providers["local"]; ok && local.LlamaServerBinary != "" {
		manager.llamaBinary = local.LlamaServerBinary
	} else if cfg.Server.LlamaServerBinary != "" {
		manager.llamaBinary = cfg.Server.LlamaServerBinary
	}
	
	return manager
}

func NewWithReapInterval(modelConfigs []models.ModelConfig, modelHost string, idleTimeout, reapInterval time.Duration) *LLMRuntimeManager {
	m := &LLMRuntimeManager{
		models:      make(map[string]models.ModelConfig),
		providers:   make(map[string]models.ProviderItem),
		idleTimeout: idleTimeout,
		modelHost:   hostFromConfig(modelHost),
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

func hostFromConfig(host string) string {
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

func (m *LLMRuntimeManager) createProviderLocked(cfg models.ModelConfig) Provider {
	// For cloud providers, we look up the credentials in the providers map
	pCfg := cfg
	var modelDir string
	if provider, ok := m.providers[cfg.Provider]; ok {
		// Merge provider settings into model config for the provider implementation
		if pCfg.ProviderConfig.APIKey == "" {
			pCfg.ProviderConfig.APIKey = provider.APIKey
		}
		if pCfg.ProviderConfig.BaseURL == "" {
			pCfg.ProviderConfig.BaseURL = provider.BaseURL
		}
		if pCfg.ProviderConfig.ProjectID == "" {
			pCfg.ProviderConfig.ProjectID = provider.ProjectID
		}
		if pCfg.ProviderConfig.Region == "" {
			pCfg.ProviderConfig.Region = provider.Region
		}
		modelDir = provider.ModelDir
	}

	switch cfg.Provider {
	case "gemini":
		return NewGeminiProvider(pCfg)
	case "vertex":
		return NewVertexProvider(pCfg)
	case "openrouter":
		return NewOpenRouterProvider(pCfg)
	case "openai":
		return NewOpenAIProvider(pCfg)
	default:
		// Fallback to local
		binary := m.llamaBinary
		if local, ok := m.providers["local"]; ok && local.LlamaServerBinary != "" {
			binary = local.LlamaServerBinary
			if modelDir == "" {
				modelDir = local.ModelDir
			}
		}
		return NewLocalProvider(pCfg, binary, modelDir)
	}
}

func (m *LLMRuntimeManager) TestProviderConnection(ctx context.Context, providerName, apiKey string) error {
	m.mu.Lock()
	cfg := models.ModelConfig{
		Provider: providerName,
		ProviderConfig: models.ProviderConfig{
			APIKey: apiKey, // Caller-supplied key takes precedence; createProviderLocked only fills empty fields
		},
	}
	p := m.createProviderLocked(cfg)
	m.mu.Unlock()
	return p.TestConnection(ctx)
}

func (m *LLMRuntimeManager) ListProviderModels(ctx context.Context, providerName string) ([]string, error) {
	m.mu.Lock()
	p := m.createProviderLocked(models.ModelConfig{Provider: providerName})
	m.mu.Unlock()
	return p.ListModels(ctx)
}

func (m *LLMRuntimeManager) EnsureModel(ctx context.Context, name string) (ModelInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.models[name]
	if !ok {
		return ModelInstance{}, ErrUnknownModel
	}

	// If it's a local model, we use the existing management logic for now
	// but we could also refactor it to use the Provider interface fully.
	if cfg.Provider == "" || cfg.Provider == "local" {
		// Existing local logic (simplified for now but preserving behavior)
		for {
			cfg = m.syncPortWithActiveLocked(cfg)
			if inst, ok := m.readyInstanceLocked(name, cfg); ok {
				return inst, nil
			}

			if m.activeModel != nil && m.activeModel.cfg.Name == name {
				return ModelInstance{}, ErrModelStarting
			}

			activePort := m.activePortLocked()
			waiter := m.signalStopLocked()
			if waiter != nil {
				m.mu.Unlock()
				waiter()
				m.mu.Lock()
				continue
			}

			cfg.Port = m.defaultPortLocked(cfg, activePort)
			m.models[name] = cfg

			err := m.startModelLocked(ctx, cfg)
			if err != nil {
				return ModelInstance{}, err
			}
			return ModelInstance{}, ErrModelStarting
		}
	}

	// For cloud providers
	if m.activeProvider == nil {
		// For now, we only support one active provider at a time
		// If switching, shutdown the old one
		if m.activeModel != nil {
			waiter := m.signalStopLocked()
			if waiter != nil {
				m.mu.Unlock()
				waiter()
				m.mu.Lock()
			}
		}
	}

	provider := m.createProviderLocked(cfg)
	if err := provider.EnsureReady(ctx); err != nil {
		return ModelInstance{}, err
	}

	url, headers, err := provider.GetEndpoint(ctx)
	if err != nil {
		return ModelInstance{}, err
	}

	m.activeProvider = provider
	m.activeCloudConfig = &cfg

	return ModelInstance{
		Name:    name,
		URL:     url,
		Headers: headers,
	}, nil
}

func (m *LLMRuntimeManager) RecordActivity(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil && m.activeModel.cfg.Name == model {
		m.activeModel.lastUsed = time.Now()
	}

	if m.activeCloudConfig != nil && m.activeCloudConfig.Name == model {
		// Just tracking it exists for now
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

func (m *LLMRuntimeManager) ClearLogs() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel != nil && m.activeModel.logs != nil {
		m.activeModel.logs.Clear()
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

func (m *LLMRuntimeManager) ActiveInfo() *ActiveModelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil {
		cfg := m.activeModel.cfg
		return &ActiveModelInfo{
			Name:     cfg.Name,
			Host:     m.modelHost,
			Port:     cfg.Port,
			Started:  m.activeModel.started,
			LastUsed: m.activeModel.lastUsed,
			Ready:    utils.PortReady(cfg.Port),
		}
	}

	if m.activeProvider != nil && m.activeCloudConfig != nil {
		return &ActiveModelInfo{
			Name:  m.activeCloudConfig.Name,
			Ready: true,
		}
	}

	return nil
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

func (m *LLMRuntimeManager) syncPortWithActiveLocked(cfg models.ModelConfig) models.ModelConfig {
	if m.activeModel != nil && cfg.Name == m.activeModel.cfg.Name {
		cfg.Port = m.activeModel.cfg.Port
	}
	return cfg
}

func (m *LLMRuntimeManager) readyInstanceLocked(name string, cfg models.ModelConfig) (ModelInstance, bool) {
	if m.activeModel != nil && m.activeModel.cfg.Name == name && utils.PortReady(m.activeModel.cfg.Port) {
		return ModelInstance{
			Name: name,
			Host: m.modelHost,
			Port: m.activeModel.cfg.Port,
		}, true
	}
	return ModelInstance{}, false
}

func (m *LLMRuntimeManager) activePortLocked() int {
	if m.activeModel != nil {
		return m.activeModel.cfg.Port
	}
	return 0
}

func (m *LLMRuntimeManager) startModelLocked(ctx context.Context, cfg models.ModelConfig) error {
	logBuf := logging.NewBufferLogger(logBufferSize)
	tokens := metrics.NewTokenTracker()
	procCtx, cancel := context.WithCancel(context.Background())

	cmd := utils.ExecCommandContext(procCtx, m.llamaBinary, buildLaunchArgs(cfg)...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Stdout = io.MultiWriter(logBuf, os.Stdout, tokens)
	cmd.Stderr = io.MultiWriter(logBuf, os.Stdout, tokens)

	if len(cfg.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("model start failed: %w", err)
	}

	m.activeModel = &runningModel{
		cfg:        cfg,
		cmd:        cmd,
		cancel:     cancel,
		started:    time.Now(),
		lastUsed:   time.Now(),
		logs:       logBuf,
		throughput: tokens,
	}

	return nil
}
