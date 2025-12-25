package proxy

import (
	"llm-proxy/internal/metrics"
	"llm-proxy/models"
	"sync"
)

type Server struct {
	manager    LLMProxyManager
	config     *models.Config
	configPath string
	modelDir   string
	gpuConfig  models.GPUConfig
	metrics    *metrics.MetricsService
	configMu   sync.Mutex
}

func NewServer(mgr LLMProxyManager, cfg *models.Config, configPath string) *Server {
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

func (s *Server) refreshMetricsService() {
	s.metrics = metrics.NewMetricsService(&models.Config{
		Metrics: models.MetricsConfig{
			GPU: s.gpuConfig,
		},
	})
	s.metrics.SetThroughputSource(s.manager)
}
