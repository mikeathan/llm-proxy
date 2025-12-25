package proxy

import (
	"llm-proxy/internal/metrics"
	"llm-proxy/models"
	"llm-proxy/utils"
	"path/filepath"
)

func (s *Server) Manager() LLMProxyManager {
	return s.manager
}

func (s *Server) ModelDir() string {
	return s.modelDir
}

func (s *Server) SetModelDir(dir string) {
	s.modelDir = dir
}

func (s *Server) GPUConfig() models.GPUConfig {
	return s.gpuConfig
}

func (s *Server) SetGPUConfig(cfg models.GPUConfig) {
	s.gpuConfig = cfg
}

func (s *Server) CurrentBinary() string {
	if s.config != nil && s.config.Server.LlamaServerBinary != "" {
		return s.config.Server.LlamaServerBinary
	}
	return "llama-server"
}

func (s *Server) CurrentIdleTimeout() int {
	if s.config != nil {
		return s.config.Server.IdleTimeoutSecs
	}
	return 0
}

func (s *Server) DefaultArgs() []string {
	if s.config == nil || len(s.config.Server.DefaultArgs) == 0 {
		return nil
	}
	return append([]string{}, s.config.Server.DefaultArgs...)
}

func (s *Server) UpdateConfig(update func(cfg *models.Config)) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	update(s.config)
	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) PersistModel(cfg models.ModelConfig) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	for _, existing := range s.config.Models {
		if existing.Name == cfg.Name {
			return nil
		}
	}

	s.config.Models = append(s.config.Models, cfg)
	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) PersistReplaceModel(cfg models.ModelConfig) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	replaced := false
	for i, m := range s.config.Models {
		if m.Name == cfg.Name {
			s.config.Models[i] = cfg
			replaced = true
			break
		}
	}
	if !replaced {
		s.config.Models = append(s.config.Models, cfg)
	}

	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) PersistDeleteModel(name string) error {
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	out := s.config.Models[:0]
	for _, m := range s.config.Models {
		if m.Name != name {
			out = append(out, m)
		}
	}
	s.config.Models = out

	return utils.SaveConfig(s.configPath, s.config)
}

func (s *Server) ResolveModelPath(filename, explicitPath string) string {
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

func (s *Server) RefreshMetricsService() {
	s.refreshMetricsService()
}

func (s *Server) MetricsSnapshot() metrics.MetricsSnapshot {
	if s.metrics == nil {
		s.refreshMetricsService()
	}
	return s.metrics.Snapshot()
}
