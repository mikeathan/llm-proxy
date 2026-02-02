package app

import (
	"errors"
	"path/filepath"
	"sync"

	"llm-proxy/internal/config"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/system_metrics"
	"llm-proxy/models"
)

type AppContext struct {
	manager   llm.RuntimeManager
	config    *models.Config
	configMgr *config.ConfigManager
	modelDir  string
	gpuConfig models.GPUConfig
	metrics   *system_metrics.MetricsService
	configMu  sync.Mutex // Kept for other fields if needed, but configMgr handles config
}

func NewServer(mgr llm.RuntimeManager, cfgMgr *config.ConfigManager) *AppContext {
	cfg := cfgMgr.GetConfig()

	s := &AppContext{
		manager:   mgr,
		config:    &cfg,
		configMgr: cfgMgr,
		modelDir:  cfg.ModelDir,
		gpuConfig: cfg.Metrics.GPU,
	}

	cfgMgr.OnChange(func(newCfg models.Config) {
		s.configMu.Lock()
		s.config = &newCfg
		s.modelDir = newCfg.ModelDir
		s.gpuConfig = newCfg.Metrics.GPU
		s.configMu.Unlock()
		s.refreshMetricsService()
	})

	s.refreshMetricsService()
	return s
}

func (a *AppContext) DefaultModel() (string, error) {
	models := a.Runtime().ListModels()
	if len(models) == 0 {
		return "", errors.New("no models configured")
	}
	return models[0].Name, nil
}

func (s *AppContext) Runtime() llm.RuntimeManager {
	return s.manager
}

func (s *AppContext) refreshMetricsService() {
	s.metrics = system_metrics.NewMetricsService(&models.Config{
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

func (s *AppContext) SetModelDir(dir string) {
	s.modelDir = dir
}

func (s *AppContext) GPUConfig() models.GPUConfig {
	return s.gpuConfig
}

func (s *AppContext) SetGPUConfig(cfg models.GPUConfig) {
	s.gpuConfig = cfg
}

func (s *AppContext) CurrentBinary() string {
	if s.config != nil && s.config.Server.LlamaServerBinary != "" {
		return s.config.Server.LlamaServerBinary
	}
	return "llama-server"
}

func (s *AppContext) CurrentIdleTimeout() int {
	if s.config != nil {
		return s.config.Server.IdleTimeoutSecs
	}
	return 0
}

func (s *AppContext) DefaultArgs() []string {
	if s.config == nil || len(s.config.Server.DefaultArgs) == 0 {
		return nil
	}
	return append([]string{}, s.config.Server.DefaultArgs...)
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

func (s *AppContext) MetricsSnapshot() system_metrics.MetricsSnapshot {
	if s.metrics == nil {
		s.refreshMetricsService()
	}
	return s.metrics.Snapshot()
}

func (s *AppContext) ListMCPServers() []models.MCPServerConfig {
	if s.config == nil {
		return nil
	}
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
