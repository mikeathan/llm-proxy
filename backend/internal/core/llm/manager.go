package llm

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"sync"
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
	logBufferSize      = 10000
)

var (
	ErrModelStarting = errors.New("model is starting")
	ErrUnknownModel  = errors.New("unknown model")
	ErrModelExists   = errors.New("model already exists")
)

type activeModelInfo struct {
	Name     string
	Provider string
	Host     string
	Port     int
	Started  time.Time
	LastUsed time.Time
	Ready    bool
}

type ModelInstance struct {
	Name    string
	ModelID string
	Host    string
	Port    int
	Path    string
	Args    []string
	URL     string
	Headers http.Header
}

type ActiveModelInfo struct {
	Name     string
	Provider string
	Host     string
	Port     int
	Started  time.Time
	LastUsed time.Time
	Ready    bool
}

type RuntimeManager interface {
	EnsureModel(ctx context.Context, name string) (ModelInstance, error)
	GetInstance(ctx context.Context, name string) (ModelInstance, error)
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
	ListProviderModels(ctx context.Context, provider, apiKeyName string) ([]string, error)
	TestProviderConnection(ctx context.Context, provider, apiKey string) error
	DefaultModel() (string, error)
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

type runningModel struct {
	cfg        models.ModelConfig
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	started    time.Time
	lastUsed   time.Time
	logs       *logging.BufferLogger
	throughput *metrics.TokenTracker
}

func (r *runningModel) Cfg() models.ModelConfig {
	return r.cfg
}

func (r *runningModel) LastUsed() time.Time {
	return r.lastUsed
}

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
	pCfg := cfg
	var modelDir string
	if provider, ok := m.providers[cfg.Provider]; ok {
		if pCfg.ProviderConfig.APIKey == "" {
			if pCfg.ProviderConfig.APIKeyName != "" {
				for _, kInfo := range provider.APIKeys {
					if kInfo.Name == pCfg.ProviderConfig.APIKeyName {
						pCfg.ProviderConfig.APIKey = kInfo.Key
						break
					}
				}
			}
			if pCfg.ProviderConfig.APIKey == "" && len(provider.APIKeys) > 0 {
				pCfg.ProviderConfig.APIKey = provider.APIKeys[0].Key
			}
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

	// Local engine is fundamentally different (binary process)
	if cfg.Provider == "local" {
		binary := m.llamaBinary
		if local, ok := m.providers["local"]; ok && local.LlamaServerBinary != "" {
			binary = local.LlamaServerBinary
			if modelDir == "" {
				modelDir = local.ModelDir
			}
		}
		return NewLocalProvider(pCfg, binary, modelDir)
	}

	// Dynamic Resolution via Manifest Registry 
	if manifest, ok := GetRegistry().Get(cfg.Provider); ok {
		if factory, ok := GetProviderFactory(manifest.Archetype); ok {
			return factory(pCfg, manifest)
		}
	}

	// Fallback/Default to local for unknown providers (keeps system resilient)
	return NewLocalProvider(pCfg, m.llamaBinary, modelDir)
}

func (m *LLMRuntimeManager) TestProviderConnection(ctx context.Context, providerName, apiKey string) error {
	m.mu.Lock()
	cfg := models.ModelConfig{
		Provider: providerName,
		ProviderConfig: models.ProviderConfig{
			APIKey: apiKey,
		},
	}
	p := m.createProviderLocked(cfg)
	m.mu.Unlock()
	return p.TestConnection(ctx)
}

func (m *LLMRuntimeManager) ListProviderModels(ctx context.Context, providerName, apiKeyName string) ([]string, error) {
	m.mu.Lock()
	p := m.createProviderLocked(models.ModelConfig{
		Provider: providerName,
		ProviderConfig: models.ProviderConfig{
			APIKeyName: apiKeyName,
		},
	})
	m.mu.Unlock()
	return p.ListModels(ctx)
}

func (m *LLMRuntimeManager) EnsureModel(ctx context.Context, name string) (ModelInstance, error) {
	inst, err := m.GetInstance(ctx, name)
	if err != nil {
		return inst, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.models[name]
	if !ok {
		return inst, nil
	}

	if cfg.Provider != "" && cfg.Provider != "local" {
		p := m.createProviderLocked(cfg)
		m.activeProvider = p
		m.activeCloudConfig = &cfg
	}

	return inst, nil
}

func (m *LLMRuntimeManager) DefaultModel() (string, error) {
	return "", nil
}

func (m *LLMRuntimeManager) GetInstance(ctx context.Context, name string) (ModelInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.models[name]
	if !ok {
		return ModelInstance{}, ErrUnknownModel
	}

	if cfg.Provider == "" || cfg.Provider == "local" {
		for {
			cfg = m.syncPortWithActiveLocked(cfg)
			if inst, ok := m.readyInstanceLocked(name, cfg); ok {
				inst.ModelID = cfg.Filename
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

	provider := m.createProviderLocked(cfg)
	if err := provider.EnsureReady(ctx); err != nil {
		return ModelInstance{}, err
	}

	url, headers, err := provider.GetEndpoint(ctx)
	if err != nil {
		return ModelInstance{}, err
	}

	return ModelInstance{
		Name:    name,
		ModelID: cfg.Filename,
		URL:     url,
		Headers: headers,
	}, nil
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

func (m *LLMRuntimeManager) ActiveInfo() *ActiveModelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil {
		cfg := m.activeModel.cfg
		return &ActiveModelInfo{
			Name:     cfg.Name,
			Provider: cfg.Provider,
			Host:     m.modelHost,
			Port:     cfg.Port,
			Started:  m.activeModel.started,
			LastUsed: m.activeModel.lastUsed,
			Ready:    portReady(cfg.Port),
		}
	}

	if m.activeProvider != nil && m.activeCloudConfig != nil {
		return &ActiveModelInfo{
			Name:     m.activeCloudConfig.Name,
			Provider: m.activeCloudConfig.Provider,
			Ready:    true,
		}
	}

	return nil
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
	if m.activeModel != nil && m.activeModel.cfg.Name == name && portReady(m.activeModel.cfg.Port) {
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

func (m *LLMRuntimeManager) ActiveModel() *runningModel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeModel
}

func (m *LLMRuntimeManager) RecordActivity(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel != nil && m.activeModel.cfg.Name == name {
		m.activeModel.lastUsed = time.Now()
	}
}

func (m *LLMRuntimeManager) ListModels() []models.ModelConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]models.ModelConfig, 0, len(m.models))
	for _, mc := range m.models {
		list = append(list, mc)
	}
	return list
}

func (m *LLMRuntimeManager) AddModel(cfg models.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.models[cfg.Name]; ok {
		return ErrModelExists
	}
	m.models[cfg.Name] = normalizeModelConfig("", cfg)
	return nil
}

func (m *LLMRuntimeManager) UpdateModel(cfg models.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models[cfg.Name] = normalizeModelConfig("", cfg)
	if m.activeModel != nil && m.activeModel.cfg.Name == cfg.Name {
		waiter := m.signalStopLocked()
		if waiter != nil {
			go waiter()
		}
	}
	return nil
}

func (m *LLMRuntimeManager) RemoveModel(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.models, name)
	if m.activeModel != nil && m.activeModel.cfg.Name == name {
		waiter := m.signalStopLocked()
		if waiter != nil {
			go waiter()
		}
	}
	return nil
}

func portReady(port int) bool {
	return utils.PortReady(port)
}
