package app

import (
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/system_metrics"
	"llm-proxy/models"
	"llm-proxy/utils"
)

type AppContext struct {
	manager    llm.RuntimeManager
	config     *models.Config
	configPath string
	modelDir   string
	gpuConfig  models.GPUConfig
	metrics    *system_metrics.MetricsService
	configMu   sync.Mutex
}

func NewServer(mgr llm.RuntimeManager, cfg *models.Config, configPath string) *AppContext {
	dir := ""
	var gpuCfg models.GPUConfig
	if cfg != nil {
		dir = cfg.ModelDir
		gpuCfg = cfg.Metrics.GPU
	}
	configPath = utils.GetAbsoluteConfigPath(configPath)

	s := &AppContext{
		manager:    mgr,
		config:     cfg,
		configPath: configPath,
		modelDir:   dir,
		gpuConfig:  gpuCfg,
	}
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

func (s *AppContext) ConfigPath() string {
	return s.configPath
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
	if s.config == nil || s.configPath == "" {
		return nil
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	update(s.config)
	return utils.SaveConfig(s.configPath, s.config)
}

func (s *AppContext) PersistModel(cfg models.ModelConfig) error {
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

func (s *AppContext) PersistReplaceModel(cfg models.ModelConfig) error {
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

func (s *AppContext) PersistDeleteModel(name string) error {
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

func BuildNodeHerder(clock utils.Clock, logger logging.Logger) nodeherder.NodeHerderService {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	baseUrl := utils.Require("DEVICE_CONTEXT_BASE_URL")
	fetcher := nodeherder.NewHttpNodeHerderFetcher(
		baseUrl,
		httpClient,
		nodeherder.NewServiceTokenManager(httpClient, baseUrl),
	)

	cache := nodeherder.NewDeviceContextCache(1*time.Hour, clock)
	return nodeherder.NewNodeHerder(fetcher, cache, logger)
}
