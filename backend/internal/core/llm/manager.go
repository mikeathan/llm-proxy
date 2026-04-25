package llm

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

const (
	defaultLlamaBinary = "llama-server"
	defaultPortStart   = models.DefaultModelPortStart
	defaultReapPeriod  = 10 * time.Second
	shutdownTimeout    = 10 * time.Second
	logBufferSize      = 10000
)

var (
	ErrUnknownModel = errors.New("unknown model")
	ErrModelExists  = errors.New("model already exists")
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
	SetModelHost(host string)
	ListProviderModels(ctx context.Context, provider, apiKeyName string) ([]string, error)
	TestProviderConnection(ctx context.Context, provider, apiKey, apiKeyName string) error
	SelectModels() (string, string)
	SetSecrets(models.SecretsStore)
	Shutdown()
	Registrar() *providers.ProviderRegistrar
}

type LLMRuntimeManager struct {
	mu                sync.Mutex
	activeModel       *providers.RunningModel
	activeProvider    models.Provider
	activeCloudConfig *models.ModelConfig
	models            map[string]models.ModelConfig
	registrar         *providers.ProviderRegistrar
	serverEnv         map[string]string
	idleTimeout       time.Duration
	stopCh            chan struct{}
}

func NewManagerFromRegistry(reg models.RegistryData, sys models.SystemConfig, settings models.UserSettings, secrets models.SecretsStore) *LLMRuntimeManager {
	logging.Info("Initializing LLM Runtime Manager from registry", "models", len(reg.Catalogue))

	registrar := providers.NewProviderRegistrar(providers.GetRegistry(), secrets, sys.Server.ModelHost)
	registrar.RegisterLocal(settings.Local.LlamaServerBinary, settings.Local.ModelDir, settings.Local.DefaultArgs)

	m := &LLMRuntimeManager{
		models:      make(map[string]models.ModelConfig),
		registrar:   registrar,
		serverEnv:   make(map[string]string),
		idleTimeout: time.Duration(sys.Server.IdleTimeoutSecs) * time.Second,
		stopCh:      make(chan struct{}),
	}

	// 1. Map Registry Catalogue to Runtime Models
	for _, entry := range reg.Catalogue {
		m.models[entry.Name] = models.ModelConfig{
			Name:     entry.Name,
			Provider: entry.ProviderID,
			Filename: entry.ModelID, // Bridge: Filename is the identifier
			Port:     entry.Port,
			Args:     entry.Args,
			ProviderConfig: &models.ProviderConfig{
				APIKeyName: entry.CredentialID,
			},
		}
	}

	go m.reapIdleModels(defaultReapPeriod)

	return m
}

func (m *LLMRuntimeManager) Registrar() *providers.ProviderRegistrar {
	return m.registrar
}

func New(modelConfigs []models.ModelConfig, modelHost string, idleTimeout time.Duration) *LLMRuntimeManager {
	return NewWithReapInterval(modelConfigs, modelHost, idleTimeout, defaultReapPeriod)
}

func NewManagerFromConfig(cfg *models.Config) *LLMRuntimeManager {
	logging.Debug("Initializing LLM Runtime Manager from legacy config")
	modelsOut := make([]models.ModelConfig, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		modelsOut = append(modelsOut, configModelFromConfig(cfg, m))
	}

	m := NewWithReapInterval(modelsOut, cfg.Server.ModelHost, time.Duration(cfg.Server.IdleTimeoutSecs)*time.Second, defaultReapPeriod)
	m.serverEnv = cfg.Server.Environment

	// Configure Registrar
	m.registrar.SetModelHost(cfg.Server.ModelHost)

	// Register Providers
	for id, p := range cfg.Providers {
		if id == "local" {
			m.registrar.RegisterLocal(p.LlamaServerBinary, p.ModelDir, p.DefaultArgs)
		} else {
			m.registrar.RegisterCloud(id, p)
		}
	}

	return m
}

func NewWithReapInterval(modelConfigs []models.ModelConfig, modelHost string, idleTimeout, reapInterval time.Duration) *LLMRuntimeManager {
	m := &LLMRuntimeManager{
		models:      make(map[string]models.ModelConfig),
		registrar:   providers.NewProviderRegistrar(providers.GetRegistry(), nil, modelHost),
		serverEnv:   make(map[string]string),
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}

	for _, mc := range modelConfigs {
		normalized := normalizeModelConfig("", mc)
		m.models[normalized.Name] = normalized
	}

	go m.reapIdleModels(reapInterval)

	return m
}

func (m *LLMRuntimeManager) TestProviderConnection(ctx context.Context, providerName, apiKey, apiKeyName string) error {
	cfg := models.ModelConfig{
		Provider: providerName,
		ProviderConfig: &models.ProviderConfig{
			APIKey:     apiKey,
			APIKeyName: apiKeyName,
		},
	}
	p, err := m.registrar.Build(cfg)
	if err != nil {
		return err
	}
	return p.TestConnection(ctx)
}

func (m *LLMRuntimeManager) ListProviderModels(ctx context.Context, providerName, apiKeyName string) ([]string, error) {
	p, err := m.registrar.Build(models.ModelConfig{
		Provider: providerName,
		ProviderConfig: &models.ProviderConfig{
			APIKeyName: apiKeyName,
		},
	})
	if err != nil {
		return nil, err
	}
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
		p, err := m.registrar.Build(cfg)
		if err == nil {
			m.activeProvider = p
			m.activeCloudConfig = &cfg
		}
	}

	return inst, nil
}

func (m *LLMRuntimeManager) SelectModels() (string, string) {
	return "", ""
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

			if m.activeModel != nil && m.activeModel.Cfg.Name == name {
				return ModelInstance{}, models.ErrModelStarting
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

			// Apply latest environment triggers (ROCm, UMA, etc.)
			startingCfg := m.enrichModelLocked(cfg)
			err := m.startModelLocked(ctx, startingCfg)
			if err != nil {
				return ModelInstance{}, err
			}
			return ModelInstance{}, models.ErrModelStarting
		}
	}

	provider, err := m.registrar.Build(cfg)
	if err != nil {
		return ModelInstance{}, err
	}
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
	if m.activeModel != nil && m.activeModel.Logs != nil {
		m.activeModel.Logs.Clear()
	}
	return nil
}

func (m *LLMRuntimeManager) ActiveInfo() *ActiveModelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeModel != nil {
		cfg := m.activeModel.Cfg
		return &ActiveModelInfo{
			Name:     cfg.Name,
			Provider: cfg.Provider,
			Host:     m.registrar.ModelHost(),
			Port:     cfg.Port,
			Started:  m.activeModel.Started,
			LastUsed: m.activeModel.LastUsed,
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
	return m.registrar.ModelHost()
}

func (m *LLMRuntimeManager) SetSecrets(s models.SecretsStore) {
	m.registrar.SetSecrets(s)
}

func (m *LLMRuntimeManager) SetModelHost(host string) {
	m.registrar.SetModelHost(host)
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
	if m.activeModel != nil && cfg.Name == m.activeModel.Cfg.Name {
		cfg.Port = m.activeModel.Cfg.Port
	}
	return cfg
}

func (m *LLMRuntimeManager) readyInstanceLocked(name string, cfg models.ModelConfig) (ModelInstance, bool) {
	if m.activeModel != nil && m.activeModel.Cfg.Name == name && portReady(m.activeModel.Cfg.Port) {
		m.activeModel.LastUsed = time.Now()
		return ModelInstance{
			Name: name,
			Host: m.registrar.ModelHost(),
			Port: m.activeModel.Cfg.Port,
		}, true
	}
	return ModelInstance{}, false
}

func (m *LLMRuntimeManager) activePortLocked() int {
	if m.activeModel != nil {
		return m.activeModel.Cfg.Port
	}
	return 0
}

func (m *LLMRuntimeManager) ActiveModel() *providers.RunningModel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeModel
}

func (m *LLMRuntimeManager) RecordActivity(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel != nil && m.activeModel.Cfg.Name == name {
		m.activeModel.LastUsed = time.Now()
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
	if m.activeModel != nil && m.activeModel.Cfg.Name == cfg.Name {
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
	if m.activeModel != nil && m.activeModel.Cfg.Name == name {
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
