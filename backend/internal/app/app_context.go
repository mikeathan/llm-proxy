package app

import (
	"path/filepath"
	"sync"

	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/models"
)

type AppContext struct {
	manager       llm.RuntimeManager
	config        models.Config
	configMgr     *config.ConfigManager
	modelDir      string
	workspacesDir string
	rootDir       string
	gpuConfig     models.GPUConfig
	metrics       *metrics.MetricsService
	configMu      sync.Mutex // Kept for other fields if needed, but configMgr handles config
}

func NewServer(mgr llm.RuntimeManager, cfgMgr *config.ConfigManager) *AppContext {
	cfg := cfgMgr.GetConfig()

	// Compute rootDir from config directory (backend/config -> repo root)
	rootDir := filepath.Dir(filepath.Dir(cfgMgr.ConfigDir()))

	local := cfg.Providers["local"]
	s := &AppContext{
		manager:       mgr,
		config:        cfg,
		configMgr:     cfgMgr,
		modelDir:      local.ModelDir,
		workspacesDir: cfg.WorkspacesDir,
		rootDir:       rootDir,
		gpuConfig:     cfg.Metrics.GPU,
	}

	// If workspaces_dir not set, default to {rootDir}/workspaces
	if s.workspacesDir == "" {
		s.workspacesDir = filepath.Join(rootDir, "workspaces")
	}

	cfgMgr.OnChange(func(newCfg models.Config) {
		s.configMu.Lock()
		s.config = newCfg
		local := newCfg.Providers["local"]
		s.modelDir = local.ModelDir
		s.gpuConfig = newCfg.Metrics.GPU
		if newCfg.WorkspacesDir != "" {
			s.workspacesDir = newCfg.WorkspacesDir
		}
		s.configMu.Unlock()
		s.refreshMetricsService()
	})

	s.refreshMetricsService()
	return s
}

func (a *AppContext) SelectModels() (string, string) {
	p := a.config.Server.PrimaryModel
	f := a.config.Server.FallbackModel

	// If no primary is set, auto-select first available local model
	if p == "" {
		models := a.Runtime().ListModels()
		if len(models) > 0 {
			p = models[0].Name
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
}

func (s *AppContext) CurrentBinary() string {
	if s.config.Server.LlamaServerBinary != "" {
		return s.config.Server.LlamaServerBinary
	}
	return "llama-server"
}

func (s *AppContext) CurrentIdleTimeout() int {
	return s.config.Server.IdleTimeoutSecs
}

func (s *AppContext) DefaultArgs() []string {
	if len(s.config.Server.DefaultArgs) == 0 {
		return nil
	}
	return append([]string{}, s.config.Server.DefaultArgs...)
}

func (s *AppContext) Environment() map[string]string {
	if s.config.Server.Environment == nil {
		return map[string]string{}
	}
	return s.config.Server.Environment
}

func (s *AppContext) Models() []models.ModelConfig {
	// return a copy
	out := make([]models.ModelConfig, len(s.config.Models))
	copy(out, s.config.Models)
	return out
}

func (s *AppContext) Providers() map[string]models.ProviderItem {
	if s.config.Providers == nil {
		return map[string]models.ProviderItem{}
	}
	out := make(map[string]models.ProviderItem, len(s.config.Providers))
	for k, v := range s.config.Providers {
		out[k] = v
	}
	return out
}

func (s *AppContext) SetEnvironment(env map[string]string) error {
	return s.UpdateConfig(func(cfg *models.Config) {
		cfg.Server.Environment = env
	})
}

func (s *AppContext) UpdateConfig(update func(cfg *models.Config)) error {
	if s.configMgr == nil {
		return nil
	}
	return s.configMgr.Update(update)
}

func (s *AppContext) PersistModel(cfg models.ModelConfig) error {
	return s.UpdateConfig(func(c *models.Config) {
		for _, existing := range c.Models {
			if existing.Name == cfg.Name {
				return
			}
		}
		c.Models = append(c.Models, cfg)
	})
}

func (s *AppContext) PersistReplaceModel(cfg models.ModelConfig) error {
	return s.UpdateConfig(func(c *models.Config) {
		replaced := false
		for i, m := range c.Models {
			if m.Name == cfg.Name {
				c.Models[i] = cfg
				replaced = true
				break
			}
		}
		if !replaced {
			c.Models = append(c.Models, cfg)
		}
	})
}

func (s *AppContext) PersistDeleteModel(name string) error {
	return s.UpdateConfig(func(c *models.Config) {
		out := c.Models[:0]
		for _, m := range c.Models {
			if m.Name != name {
				out = append(out, m)
			}
		}
		c.Models = out
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
	// Return copy
	return append([]models.MCPServerConfig{}, s.config.MCPServers...)
}

func (s *AppContext) AddMCPServer(cfg models.MCPServerConfig) error {
	return s.UpdateConfig(func(c *models.Config) {
		for _, existing := range c.MCPServers {
			if existing.Name == cfg.Name {
				// Already exists
				return
			}
		}
		c.MCPServers = append(c.MCPServers, cfg)
	})
}

func (s *AppContext) UpdateMCPServer(cfg models.MCPServerConfig) error {
	return s.UpdateConfig(func(c *models.Config) {
		for i, m := range c.MCPServers {
			if m.Name == cfg.Name {
				c.MCPServers[i] = cfg
				return
			}
		}
	})
}

func (s *AppContext) RemoveMCPServer(name string) error {
	return s.UpdateConfig(func(c *models.Config) {
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
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return &s.config
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
	// Note: logging.NewFileLogger handles directory creation
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
