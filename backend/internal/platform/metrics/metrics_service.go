package metrics

import (
	"llm-proxy/models"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

func NewMetricsService(cfg *models.Config) *MetricsService {
	provider, name, initErr := buildGPUProvider(cfg)
	interval := 0
	if cfg != nil {
		interval = cfg.Metrics.GPUSampleIntervalSec
	}
	return &MetricsService{
		gpu:               provider,
		gpuProviderName:   name,
		gpuInitErr:        initErr,
		nowFn:             time.Now,
		gpuSampleInterval: time.Duration(interval) * time.Second,
	}
}

func (s *MetricsService) SetThroughputSource(src ThroughputSource) {
	s.throughput = src
}

func (s *MetricsService) SetTerminalSource(src TerminalSource) {
	s.terminal = src
}

func (s *MetricsService) Snapshot() MetricsSnapshot {
	host := s.readHostMetrics()
	resp := MetricsSnapshot{
		HostMetrics: host,
		GPUProvider: s.gpuProviderName,
	}

	if s.gpuInitErr != "" {
		resp.GPUError = s.gpuInitErr
		return resp
	}

	if s.gpu == nil {
		return resp
	}

	gpu, err := s.readGPU()
	if err != nil {
		resp.GPUError = err.Error()
		return resp
	}

	s.gpuMu.RLock()
	cachedAt := s.gpuCachedAt
	s.gpuMu.RUnlock()
	if !cachedAt.IsZero() {
		age := s.nowFn().Sub(cachedAt).Seconds()
		resp.GPUCacheAgeSec = age
		if s.gpuSampleInterval > 0 && age > s.gpuSampleInterval.Seconds()*2 {
			resp.GPUStale = true
		}
	}

	resp.GPU = gpu
	resp.GPUCorePercent = gpu.UtilizationPct
	resp.GPUMemoryPercent = gpu.MemoryUtilizationPct
	resp.GPUMemoryUsedMB = gpu.MemoryUsedMB
	resp.GPUMemoryTotalMB = gpu.MemoryTotalMB

	if gpu.GttUsedMB > 0 {
		resp.HostMetrics.MemUsedMB -= gpu.GttUsedMB
		if resp.HostMetrics.MemUsedMB < 0 {
			resp.HostMetrics.MemUsedMB = 0
		}
	}

	if s.throughput != nil {
		if tps, ts := s.throughput.LastTokensPerSecond(); tps > 0 {
			resp.LLMTokensPerSec = tps
			resp.LLMTokensPerSecTS = ts
		}
	}

	if s.terminal != nil {
		idle, active := s.terminal.HealthCheck()
		resp.IdleTerminals = idle
		resp.ActiveTerminals = active
	}
	return resp
}

func (s *MetricsService) Start() {
	if s.gpu == nil || s.gpuSampleInterval <= 0 {
		return
	}
	s.gpuMu.Lock()
	if s.stopCh != nil {
		s.gpuMu.Unlock()
		return
	}
	s.stopCh = make(chan struct{})
	stop := s.stopCh
	s.gpuMu.Unlock()

	gpu, err := s.gpu.Sample()
	s.gpuMu.Lock()
	s.gpuCached = gpu
	s.gpuCachedErr = err
	s.gpuCachedAt = s.nowFn()
	s.gpuMu.Unlock()

	go func() {
		ticker := time.NewTicker(s.gpuSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				gpu, err := s.gpu.Sample()
				s.gpuMu.Lock()
				s.gpuCached = gpu
				s.gpuCachedErr = err
				s.gpuCachedAt = s.nowFn()
				s.gpuMu.Unlock()
			case <-stop:
				return
			}
		}
	}()
}

func (s *MetricsService) Stop() {
	s.gpuMu.Lock()
	ch := s.stopCh
	s.stopCh = nil
	s.gpuMu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func (s *MetricsService) readGPU() (*GPUMetrics, error) {
	s.gpuMu.RLock()
	cached := s.gpuCached
	cachedErr := s.gpuCachedErr
	hasCache := !s.gpuCachedAt.IsZero()
	s.gpuMu.RUnlock()

	if hasCache {
		return cached, cachedErr
	}
	return s.gpu.Sample()
}

func (s *MetricsService) readHostMetrics() HostMetrics {
	loadAvg, _ := load.Avg()
	vmem, _ := mem.VirtualMemory()
	cores := runtime.NumCPU()

	// Calculate Instantaneous CPU Utilization
	// We use cpu.Percent with 0 to get usage since last call
	percents, err := cpu.Percent(0, false)
	pct := 0.0
	if err == nil && len(percents) > 0 {
		pct = percents[0]
	}

	m := HostMetrics{
		Cores:     cores,
		Timestamp: s.nowFn(),
		LoadPct:   pct,
	}

	if loadAvg != nil {
		m.Load1 = loadAvg.Load1
		m.Load5 = loadAvg.Load5
		m.Load15 = loadAvg.Load15
	}

	if vmem != nil {
		m.MemTotalMB = float64(vmem.Total) / 1024 / 1024
		m.MemFreeMB = float64(vmem.Free) / 1024 / 1024
		m.MemAvailableMB = float64(vmem.Available) / 1024 / 1024
		m.MemUsedMB = float64(vmem.Used) / 1024 / 1024
	}

	return m
}

func (s *MetricsService) GPUProvider() string {
	return s.gpuProviderName
}

func (s *MetricsService) TerminalSource() TerminalSource {
	return s.terminal
}
