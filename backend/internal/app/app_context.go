package app

import (
	"fmt"
	"path/filepath"
	"sync"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/secrets"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type AppContext struct {
	manager       llm.RuntimeManager
	dataMgr       *storage.DataManager
	modelDir      string
	workspacesDir string
	rootDir       string
	gpuConfig     models.GPUConfig
	metrics       *metrics.MetricsService
	configMu      sync.RWMutex
}

func NewServer(mgr llm.RuntimeManager, dataMgr *storage.DataManager) *AppContext {
	logging.Info("Initializing AppContext server state")

	sys := dataMgr.System().Get()
	rootDir := dataMgr.RootDir()

	s := &AppContext{
		manager:       mgr,
		dataMgr:       dataMgr,
		modelDir:      sys.Local.ModelDir,
		workspacesDir: dataMgr.WorkspacesDir(),
		rootDir:       rootDir,
	}

	// Link manager to secrets
	if m, ok := mgr.(*llm.LLMRuntimeManager); ok {
		m.SetSecrets(storage.NewSecretsBridge(dataMgr.Secrets()))
	}

	logging.Debug("State initialized",
		"root", rootDir,
		"workspaces", s.workspacesDir,
		"model_dir", s.modelDir)

	s.refreshMetricsService()
	return s
}

func (a *AppContext) SelectModels() (string, string) {
	sys := a.dataMgr.System().Get()
	p := sys.Server.PrimaryModel
	f := sys.Server.FallbackModel

	// If no primary is set, auto-select first available model from registry
	if p == "" {
		reg := a.dataMgr.Registry().Get()
		if len(reg.Catalogue) > 0 {
			p = reg.Catalogue[0].Name
		}
	}

	return p, f
}

func (s *AppContext) Runtime() llm.RuntimeManager {
	return s.manager
}

func (s *AppContext) refreshMetricsService() {
	s.metrics = metrics.NewMetricsService(&models.Config{
		Metrics: models.MetricsConfig{
			GPU: s.gpuConfig,
		},
	})
	s.metrics.SetThroughputSource(s.manager)
}

func (s *AppContext) Manager() llm.RuntimeManager {
	return s.manager
}

func (s *AppContext) Secrets() secrets.Store {
	return storage.NewSecretsBridge(s.dataMgr.Secrets())
}

func (s *AppContext) ModelDir() string {
	return s.modelDir
}

func (s *AppContext) RootDir() string {
	return s.rootDir
}

func (s *AppContext) WorkspacesDir() string {
	return s.workspacesDir
}

func (s *AppContext) SetModelDir(dir string) {
	s.modelDir = dir
}

func (s *AppContext) SetWorkspacesDir(dir string) {
	s.workspacesDir = dir
}

func (s *AppContext) GPUConfig() models.GPUConfig {
	return s.gpuConfig
}

func (s *AppContext) SetGPUConfig(cfg models.GPUConfig) {
	s.gpuConfig = cfg
	s.refreshMetricsService()
}

func (s *AppContext) CurrentBinary() string {
	sys := s.dataMgr.System().Get()
	if sys.Local.LlamaServerBinary != "" {
		return sys.Local.LlamaServerBinary
	}
	return "llama-server"
}

func (s *AppContext) CurrentIdleTimeout() int {
	sys := s.dataMgr.System().Get()
	return sys.Server.IdleTimeoutSecs
}

func (s *AppContext) DefaultArgs() []string {
	sys := s.dataMgr.System().Get()
	if len(sys.Local.DefaultArgs) == 0 {
		return nil
	}
	return append([]string{}, sys.Local.DefaultArgs...)
}

func (s *AppContext) Environment() map[string]string {
	return map[string]string{}
}

func (s *AppContext) Models() []models.ModelConfig {
	reg := s.dataMgr.Registry().Get()
	out := make([]models.ModelConfig, len(reg.Catalogue))
	for i, m := range reg.Catalogue {
		out[i] = models.ModelConfig{
			Name:     m.Name,
			Provider: m.ProviderID,
			Filename: m.ModelID,
			ProviderConfig: models.ProviderConfig{
				APIKeyName: m.CredentialID,
			},
		}
	}
	return out
}

func (s *AppContext) Providers() map[string]models.ProviderItem {
	reg := s.dataMgr.Registry().Get()
	out := make(map[string]models.ProviderItem, len(reg.Providers))
	for k, v := range reg.Providers {
		out[k] = models.ProviderItem{
			Type:    v.Type,
			BaseURL: v.BaseURL,
		}
	}
	return out
}

func (s *AppContext) SetEnvironment(env map[string]string) error {
	return nil
}

func (s *AppContext) SyncGuardrails(cfg models.AgentGuardrailsConfig) error {
	var errs []error
	if err := tools.SaveManifest(s.rootDir, "terminal", cfg.Terminal); err != nil {
		errs = append(errs, fmt.Errorf("terminal: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, "filesystem", cfg.FileSystem); err != nil {
		errs = append(errs, fmt.Errorf("filesystem: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, "search", cfg.Search); err != nil {
		errs = append(errs, fmt.Errorf("search: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, "communication", cfg.Communication); err != nil {
		errs = append(errs, fmt.Errorf("communication: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, "security", cfg.Global); err != nil {
		errs = append(errs, fmt.Errorf("security: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync failed: %v", errs)
	}
	return nil
}

func (a *AppContext) UpdateConfig(fn func(*models.Config)) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.Config()
	fn(cfg)

	// Persist Registry parts
	err := a.dataMgr.Registry().Update(func(reg *storage.RegistryData) {
		reg.Catalogue = make([]storage.ModelRegistryEntry, 0, len(cfg.Models))
		for _, m := range cfg.Models {
			reg.Catalogue = append(reg.Catalogue, storage.ModelRegistryEntry{
				Name:         m.Name,
				ProviderID:   m.Provider,
				ModelID:      m.Filename,
				CredentialID: m.ProviderConfig.APIKeyName,
			})
		}
		reg.MCPServers = make([]storage.MCPServerRegistryEntry, 0, len(cfg.MCPServers))
		for _, s := range cfg.MCPServers {
			reg.MCPServers = append(reg.MCPServers, storage.MCPServerRegistryEntry{
				Name:    s.Name,
				URL:     s.URL,
				Enabled: s.Enabled,
			})
		}
	})
	if err != nil {
		return err
	}

	// Persist System parts
	return a.dataMgr.System().Update(func(sys *storage.SystemConfig) {
		sys.Server.Bind = cfg.Server.Bind
		sys.Server.ModelHost = cfg.Server.ModelHost
		sys.Server.IdleTimeoutSecs = cfg.Server.IdleTimeoutSecs
		sys.Server.PrimaryModel = cfg.Server.PrimaryModel
		sys.Server.FallbackModel = cfg.Server.FallbackModel
		sys.WorkspacesDir = cfg.WorkspacesDir
		sys.Local.DefaultArgs = cfg.Server.DefaultArgs
	})
}

func (s *AppContext) PersistModel(cfg models.ModelConfig) error {
	logging.Info("Persisting new model to registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *storage.RegistryData) {
		for _, existing := range c.Catalogue {
			if existing.Name == cfg.Name {
				return
			}
		}
		c.Catalogue = append(c.Catalogue, storage.ModelRegistryEntry{
			ID:           cfg.Name,
			Name:         cfg.Name,
			ProviderID:   cfg.Provider,
			ModelID:      cfg.Filename,
			CredentialID: cfg.ProviderConfig.APIKeyName,
		})
	})
}

func (s *AppContext) PersistReplaceModel(cfg models.ModelConfig) error {
	logging.Info("Replacing model in registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *storage.RegistryData) {
		replaced := false
		newEntry := storage.ModelRegistryEntry{
			ID:           cfg.Name,
			Name:         cfg.Name,
			ProviderID:   cfg.Provider,
			ModelID:      cfg.Filename,
			CredentialID: cfg.ProviderConfig.APIKeyName,
		}
		for i, m := range c.Catalogue {
			if m.Name == cfg.Name {
				c.Catalogue[i] = newEntry
				replaced = true
				break
			}
		}
		if !replaced {
			c.Catalogue = append(c.Catalogue, newEntry)
		}
	})
}

func (s *AppContext) PersistDeleteModel(name string) error {
	logging.Info("Deleting model from registry", "name", name)
	return s.dataMgr.Registry().Update(func(c *storage.RegistryData) {
		out := c.Catalogue[:0]
		for _, m := range c.Catalogue {
			if m.Name != name {
				out = append(out, m)
			}
		}
		c.Catalogue = out
	})
}

func (s *AppContext) ResolveModelPath(filename, explicitPath string) string {
	if explicitPath != "" && filepath.IsAbs(explicitPath) {
		return explicitPath
	}
	if filename == "" && explicitPath != "" {
		return explicitPath
	}
	if filepath.IsAbs(filename) {
		return filename
	}
	if s.modelDir != "" {
		return filepath.Join(s.modelDir, filename)
	}
	if explicitPath != "" {
		return explicitPath
	}
	return filename
}

func (s *AppContext) RefreshMetricsService() {
	s.refreshMetricsService()
}

func (s *AppContext) MetricsSnapshot() metrics.MetricsSnapshot {
	if s.metrics == nil {
		s.refreshMetricsService()
	}
	return s.metrics.Snapshot()
}

func (s *AppContext) ListMCPServers() []models.MCPServerConfig {
	reg := s.dataMgr.Registry().Get()
	out := make([]models.MCPServerConfig, len(reg.MCPServers))
	for i, s := range reg.MCPServers {
		out[i] = models.MCPServerConfig{
			Name:    s.Name,
			URL:     s.URL,
			Enabled: s.Enabled,
		}
	}
	return out
}

func (s *AppContext) AddMCPServer(cfg models.MCPServerConfig) error {
	logging.Info("Adding new MCP server to registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *storage.RegistryData) {
		for _, existing := range c.MCPServers {
			if existing.Name == cfg.Name {
				return
			}
		}
		c.MCPServers = append(c.MCPServers, storage.MCPServerRegistryEntry{
			Name:    cfg.Name,
			URL:     cfg.URL,
			Enabled: cfg.Enabled,
		})
	})
}

func (s *AppContext) UpdateMCPServer(cfg models.MCPServerConfig) error {
	logging.Info("Updating MCP server in registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *storage.RegistryData) {
		for i, m := range c.MCPServers {
			if m.Name == cfg.Name {
				c.MCPServers[i] = storage.MCPServerRegistryEntry{
					Name:    cfg.Name,
					URL:     cfg.URL,
					Enabled: cfg.Enabled,
				}
				return
			}
		}
	})
}

func (s *AppContext) RemoveMCPServer(name string) error {
	logging.Info("Removing MCP server from registry", "name", name)
	return s.dataMgr.Registry().Update(func(c *storage.RegistryData) {
		out := c.MCPServers[:0]
		for _, m := range c.MCPServers {
			if m.Name != name {
				out = append(out, m)
			}
		}
		c.MCPServers = out
	})
}

func (s *AppContext) Config() *models.Config {
	sys := s.dataMgr.System().Get()
	reg := s.dataMgr.Registry().Get()

	cfg := &models.Config{
		Server: models.ServerConfig{
			Bind:            sys.Server.Bind,
			ModelHost:       sys.Server.ModelHost,
			IdleTimeoutSecs: sys.Server.IdleTimeoutSecs,
			PrimaryModel:    sys.Server.PrimaryModel,
			FallbackModel:   sys.Server.FallbackModel,
			DefaultArgs:     sys.Local.DefaultArgs,
			LlamaServerBinary: sys.Local.LlamaServerBinary,
		},
		Models: s.Models(),
	}

	cfg.Providers = make(map[string]models.ProviderItem)
	configLocal := sys.Local
	cfg.Providers["local"] = models.ProviderItem{
		Type:              "local",
		LlamaServerBinary: configLocal.LlamaServerBinary,
		ModelDir:          configLocal.ModelDir,
		DefaultArgs:       configLocal.DefaultArgs,
	}

	for k, v := range reg.Providers {
		cfg.Providers[k] = models.ProviderItem{
			Type:    v.Type,
			BaseURL: v.BaseURL,
		}
	}

	cfg.MCPServers = s.ListMCPServers()
	cfg.WorkspacesDir = s.workspacesDir

	return cfg
}

func (s *AppContext) ProcessLogger(workspaceID string) logging.Logger {
	if workspaceID == "" {
		return logging.GetGlobalLogger()
	}
	dir := s.workspacesDir
	if dir == "" {
		dir = "workspaces"
	}
	logFile := filepath.Join(dir, workspaceID, ".internal", "process.log")
	l, err := logging.NewFileLogger(logging.Options{
		File:   logFile,
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		return logging.GetGlobalLogger()
	}
	return l
}
