package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/env"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type AppContext struct {
	manager       llm.RuntimeManager
	dataMgr       *storage.DataManager
	modelDir      string
	resolver      *storage.PathResolver
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
		manager:   mgr,
		dataMgr:   dataMgr,
		modelDir:  sys.Local.ModelDir,
		resolver:  storage.NewPathResolver(dataMgr.WorkspacesDir()),
		rootDir:   rootDir,
		gpuConfig: sys.Metrics.GPU,
	}

	// Link manager to secrets
	if m, ok := mgr.(*llm.LLMRuntimeManager); ok {
		m.SetSecrets(dataMgr.Secrets())
	}

	logging.Debug("State initialized",
		"root", rootDir,
		"workspaces", s.resolver.WorkspacesRoot(),
		"model_dir", s.modelDir)

	s.refreshMetricsService()
	return s
}

func (a *AppContext) SelectModels() (string, string) {
	reg := a.dataMgr.Registry().Get()
	p := reg.PrimaryModel
	f := reg.FallbackModel

	// If no primary is set, auto-select first available model from registry
	if p == "" {
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

func (s *AppContext) Secrets() models.SecretsStore {
	return s.dataMgr.Secrets()
}

func (s *AppContext) ModelDir() string {
	return s.modelDir
}

func (s *AppContext) RootDir() string {
	return s.rootDir
}

func (s *AppContext) WorkspacesDir() string {
	return s.resolver.WorkspacesRoot()
}

func (s *AppContext) SetModelDir(dir string) {
	s.modelDir = dir
}

func (s *AppContext) SetWorkspacesDir(dir string) {
	s.resolver = storage.NewPathResolver(dir)
}

// Tier 1: System
func (s *AppContext) GetSystem() models.SystemConfig {
	return s.dataMgr.System().Get()
}

func (s *AppContext) UpdateSystem(fn func(*models.SystemConfig)) error {
	return s.dataMgr.System().Update(fn)
}

// Tier 2: Registry
func (s *AppContext) GetRegistry() models.RegistryData {
	return s.dataMgr.Registry().Get()
}

func (s *AppContext) UpdateRegistry(fn func(*models.RegistryData)) error {
	return s.dataMgr.Registry().Update(fn)
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
	sys := s.dataMgr.System().Get()
	out := make(map[string]models.ProviderItem)

	// 1. Populate from Registry
	for k, v := range reg.Providers {
		out[k] = models.ProviderItem{
			Type:    v.Type,
			BaseURL: v.BaseURL,
		}
	}

	// 2. Ensure 'local' is present and enriched from system config
	local := out["local"]
	if local.Type == "" {
		local.Type = "local"
	}
	local.LlamaServerBinary = sys.Local.LlamaServerBinary
	local.ModelDir = sys.Local.ModelDir
	local.DefaultArgs = sys.Local.DefaultArgs
	out["local"] = local

	return out
}

func (s *AppContext) SetEnvironment(env map[string]string) error {
	return nil
}

func (s *AppContext) SyncGuardrails(cfg models.AgentGuardrailsConfig) error {
	var errs []error
	if err := tools.SaveManifest(s.rootDir, models.CategoryTerminal, cfg.Terminal); err != nil {
		errs = append(errs, fmt.Errorf("terminal: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, models.CategoryFileSystem, cfg.FileSystem); err != nil {
		errs = append(errs, fmt.Errorf("filesystem: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, models.CategorySearch, cfg.Search); err != nil {
		errs = append(errs, fmt.Errorf("search: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, models.CategoryCommunication, cfg.Communication); err != nil {
		errs = append(errs, fmt.Errorf("communication: %w", err))
	}
	if err := tools.SaveManifest(s.rootDir, models.CategoryGlobal, cfg.Global); err != nil {
		errs = append(errs, fmt.Errorf("security: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync failed: %v", errs)
	}
	return nil
}

func (s *AppContext) UpdateSettings(ctx context.Context, req models.SystemUpdatePayload) error {
	// 1. Update Infrastructure (SystemConfig)
	err := s.dataMgr.System().Update(func(sys *models.SystemConfig) {
		if req.Bind != "" {
			sys.Server.Bind = req.Bind
		}
		if req.WorkspacesDir != "" {
			sys.WorkspacesDir = req.WorkspacesDir
			s.SetWorkspacesDir(req.WorkspacesDir)
		}
		if req.ModelHost != "" {
			sys.Server.ModelHost = req.ModelHost
		}
		if req.IdleTimeoutSecs > 0 {
			sys.Server.IdleTimeoutSecs = req.IdleTimeoutSecs
		}
		if req.Environment != nil {
			sys.Server.Environment = req.Environment
		}
		if req.DefaultArgs != nil {
			sys.Local.DefaultArgs = req.DefaultArgs
		}

		// Sync 'local' infrastructure fields
		if local, ok := req.Providers["local"]; ok {
			if local.LlamaServerBinary != "" {
				sys.Local.LlamaServerBinary = local.LlamaServerBinary
			}
			if local.ModelDir != "" {
				sys.Local.ModelDir = local.ModelDir
				s.modelDir = local.ModelDir
			}
			if local.DefaultArgs != nil {
				sys.Local.DefaultArgs = local.DefaultArgs
			}
		}
		// 2. Sync GPU Configuration (moved into transaction for persistence)
		if req.GPUProvider != "" {
			sys.Metrics.GPU.Provider = req.GPUProvider
		}
		if req.GPUBinary != "" {
			sys.Metrics.GPU.Binary = req.GPUBinary
		}
		if req.GPUIndex != nil {
			sys.Metrics.GPU.Index = *req.GPUIndex
		}
		if req.GPUSysfsPath != "" {
			sys.Metrics.GPU.SysfsPath = req.GPUSysfsPath
		}
	})
	if err != nil {
		return fmt.Errorf("failed to save system config: %w", err)
	}

	// 2. Refresh in-memory state from the newly saved config
	newSys := s.dataMgr.System().Get()
	s.SetGPUConfig(newSys.Metrics.GPU)

	// 3. Update Registry Providers
	if req.Providers != nil {
		err = s.dataMgr.Registry().Update(func(reg *models.RegistryData) {
			if reg.Providers == nil {
				reg.Providers = make(map[string]models.ProviderRegistryEntry)
			}
			for id, p := range req.Providers {
				if id == "local" {
					continue // Infrastructure-only
				}
				entry := reg.Providers[id]
				entry.Type = p.Type
				entry.BaseURL = p.BaseURL
				reg.Providers[id] = entry
			}
			if req.FallbackModel != "" {
				reg.FallbackModel = req.FallbackModel
			}
			if req.Communication != nil {
				reg.Communication = *req.Communication
			}
			if req.Search != nil {
				reg.Search = *req.Search
			}
		})
		if err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	} else if req.PrimaryModel != "" || req.FallbackModel != "" || req.Communication != nil || req.Search != nil {
		// Just update registry items if providers weren't involved
		err = s.dataMgr.Registry().Update(func(reg *models.RegistryData) {
			if req.PrimaryModel != "" {
				reg.PrimaryModel = req.PrimaryModel
			}
			if req.FallbackModel != "" {
				reg.FallbackModel = req.FallbackModel
			}
			if req.Communication != nil {
				reg.Communication = *req.Communication
			}
			if req.Search != nil {
				reg.Search = *req.Search
			}
		})
		if err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
	}

	// 4. Update Runtime (ModelHost)
	if req.ModelHost != "" {
		s.manager.SetModelHost(req.ModelHost)
	}

	// 5. Update Environment Variables (.env)
	envUpdates := map[string]string{}
	if req.ServiceClientID != "" {
		os.Setenv("SERVICE_CLIENT_ID", req.ServiceClientID)
		envUpdates["SERVICE_CLIENT_ID"] = req.ServiceClientID
	}
	if req.ServiceClientSecret != "" {
		os.Setenv("SERVICE_CLIENT_SECRET", req.ServiceClientSecret)
		envUpdates["SERVICE_CLIENT_SECRET"] = req.ServiceClientSecret
	}
	if len(envUpdates) > 0 {
		envPath, _ := env.EnvFilePaths()
		_ = env.UpdateEnvFile(envPath, envUpdates)
	}

	// 6. Push env to active models
	if req.Environment != nil {
		for _, m := range s.manager.ListModels() {
			m.Environment = req.Environment
			_ = s.manager.UpdateModel(m)
		}
	}

	// 7. Sync Guardrails
	if req.Guardrails != nil {
		if err := s.SyncGuardrails(*req.Guardrails); err != nil {
			return fmt.Errorf("failed to sync guardrails: %w", err)
		}
	}

	return nil
}

func (s *AppContext) ServiceCredentials() (id, secret string) {
	return os.Getenv("SERVICE_CLIENT_ID"), os.Getenv("SERVICE_CLIENT_SECRET")
}

func (s *AppContext) PersistModel(cfg models.ModelConfig) error {
	logging.Info("Persisting new model to registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) {
		for _, existing := range c.Catalogue {
			if existing.Name == cfg.Name {
				return
			}
		}
		c.Catalogue = append(c.Catalogue, models.ModelRegistryEntry{
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
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) {
		replaced := false
		newEntry := models.ModelRegistryEntry{
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
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) {
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
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) {
		for _, existing := range c.MCPServers {
			if existing.Name == cfg.Name {
				return
			}
		}
		c.MCPServers = append(c.MCPServers, models.MCPServerRegistryEntry{
			Name:    cfg.Name,
			URL:     cfg.URL,
			Enabled: cfg.Enabled,
		})
	})
}

func (s *AppContext) UpdateMCPServer(cfg models.MCPServerConfig) error {
	logging.Info("Updating MCP server in registry", "name", cfg.Name)
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) {
		for i, m := range c.MCPServers {
			if m.Name == cfg.Name {
				c.MCPServers[i] = models.MCPServerRegistryEntry{
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
	return s.dataMgr.Registry().Update(func(c *models.RegistryData) {
		out := c.MCPServers[:0]
		for _, m := range c.MCPServers {
			if m.Name != name {
				out = append(out, m)
			}
		}
		c.MCPServers = out
	})
}

func (s *AppContext) ProcessLogger(workspaceID string) logging.Logger {
	if workspaceID == "" {
		return logging.GetGlobalLogger()
	}

	logFile := s.resolver.ProcessLog(workspaceID)
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
func (s *AppContext) ListTemplates() ([]models.TemplateMetadata, error) {
	return s.dataMgr.Templates().List()
}

func (s *AppContext) GetTemplate(id string) (models.Template, error) {
	return s.dataMgr.Templates().Get(id)
}
