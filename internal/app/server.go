package app

import (
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"llm-proxy/internal/device_context"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/system_metrics"
	"llm-proxy/models"
	"llm-proxy/utils"
)

type Server struct {
	manager    llm.RuntimeManager
	config     *models.Config
	configPath string
	modelDir   string
	gpuConfig  models.GPUConfig
	metrics    *system_metrics.MetricsService
	configMu   sync.Mutex
}

func NewServer(mgr llm.RuntimeManager, cfg *models.Config, configPath string) *Server {
	dir := ""
	var gpuCfg models.GPUConfig
	if cfg != nil {
		dir = cfg.ModelDir
		gpuCfg = cfg.Metrics.GPU
	}

	s := &Server{
		manager:    mgr,
		config:     cfg,
		configPath: configPath,
		modelDir:   dir,
		gpuConfig:  gpuCfg,
	}
	s.refreshMetricsService()
	return s
}

func (s *Server) Runtime() llm.RuntimeManager {
	return s.manager
}

func (s *Server) refreshMetricsService() {
	s.metrics = system_metrics.NewMetricsService(&models.Config{
		Metrics: models.MetricsConfig{
			GPU: s.gpuConfig,
		},
	})
	s.metrics.SetThroughputSource(s.manager)
}

func (s *Server) Manager() llm.RuntimeManager {
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

func (s *Server) MetricsSnapshot() system_metrics.MetricsSnapshot {
	if s.metrics == nil {
		s.refreshMetricsService()
	}
	return s.metrics.Snapshot()
}

func BuildDeviceContextProvider(clock utils.Clock) device_context.DeviceContextProvider {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	fetcher := device_context.NewHttpDeviceContextFetcher(
		utils.Require("DEVICE_CONTEXT_BASE_URL"),
		httpClient,
	)

	cache := device_context.NewDeviceContextCache(1*time.Hour, clock)
	return device_context.NewDeviceContextProvider(fetcher, cache)
}
