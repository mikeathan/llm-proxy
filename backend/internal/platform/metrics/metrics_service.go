package metrics

import (
	"llm-proxy/models"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

const defaultGPUSmoothingAlpha = 0.3

const gpuSeedSamples = 3

func NewMetricsService(cfg *models.Config) *MetricsService {
	provider, name, initErr := buildGPUProvider(cfg)
	interval := 0
	if cfg != nil {
		interval = cfg.Metrics.GPUSampleIntervalSec
	}
	alpha := 0.0
	if cfg != nil {
		alpha = cfg.Metrics.GPUSmoothingAlpha
	}
	return &MetricsService{
		gpu:               provider,
		gpuProviderName:   name,
		gpuInitErr:        initErr,
		nowFn:             time.Now,
		gpuSampleInterval: time.Duration(interval) * time.Second,
		gpuSmoothingAlpha: alpha,
	}
}

func (s *MetricsService) effectiveSmoothingAlpha() float64 {
	s.gpuMu.RLock()
	a := s.gpuSmoothingAlpha
	s.gpuMu.RUnlock()
	if a <= 0 || a > 1 {
		return defaultGPUSmoothingAlpha
	}
	return a
}

func (s *MetricsService) smoothGPU(sample *GPUMetrics) *GPUMetrics {
	if sample == nil {
		return nil
	}
	alpha := s.effectiveSmoothingAlpha()

	s.gpuMu.Lock()
	if s.gpuSeedCount < gpuSeedSamples {
		s.gpuUtilAvg += (sample.UtilizationPct - s.gpuUtilAvg) / float64(s.gpuSeedCount+1)
		s.gpuMemAvg += (sample.MemoryUtilizationPct - s.gpuMemAvg) / float64(s.gpuSeedCount+1)
		s.gpuSeedCount++
	} else {
		s.gpuUtilAvg += alpha * (sample.UtilizationPct - s.gpuUtilAvg)
		s.gpuMemAvg += alpha * (sample.MemoryUtilizationPct - s.gpuMemAvg)
	}
	util := s.gpuUtilAvg
	mem := s.gpuMemAvg
	s.gpuMu.Unlock()

	out := *sample
	out.UtilizationPct = util
	out.MemoryUtilizationPct = mem
	return &out
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

// sampleAndSmooth runs one provider sample and applies the EMA smoothing.
// Shared by the background sampler (Start) and the on-demand fallback
// (readGPU); callers guard against a nil provider before calling.
func (s *MetricsService) sampleAndSmooth() (*GPUMetrics, error) {
	sample, err := s.gpu.Sample()
	if err != nil {
		return nil, err
	}
	return s.smoothGPU(sample), nil
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

	gpu, err := s.sampleAndSmooth()
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
				gpu, err := s.sampleAndSmooth()
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
	return s.sampleAndSmooth()
}

// hostMetricsCacheInterval bounds how often host metrics are re-read. Several
// frontend components poll /metrics on a 10s cadence; caching also serializes
// cpu.Percent(0) — a "since the last call" delta that is not safe under
// concurrent callers (ops review 2026-08-28 finding #3).
const hostMetricsCacheInterval = 2 * time.Second

// readHostMetrics returns cached host metrics when fresh, otherwise recomputes
// them under the mutex so concurrent snapshot requests never race cpu.Percent.
func (s *MetricsService) readHostMetrics() HostMetrics {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	if !s.hostCachedAt.IsZero() && s.nowFn().Sub(s.hostCachedAt) < hostMetricsCacheInterval {
		return s.hostCached
	}
	s.hostCached = s.computeHostMetrics()
	s.hostCachedAt = s.nowFn()
	return s.hostCached
}

func (s *MetricsService) computeHostMetrics() HostMetrics {
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

func (s *MetricsService) SetSmoothingAlpha(a float64) {
	s.gpuMu.Lock()
	s.gpuSmoothingAlpha = a
	s.gpuMu.Unlock()
}

func (s *MetricsService) GPUProvider() string {
	return s.gpuProviderName
}

// SampleInterval returns the configured background GPU sample cadence. 0 means
// no background sampler is running — sampling is on-demand per Snapshot().
// Read-only after construction (no lock needed).
func (s *MetricsService) SampleInterval() time.Duration {
	return s.gpuSampleInterval
}

// EffectiveSmoothingAlpha returns the smoothing alpha in effect: the configured
// value, or the default when unset/invalid.
func (s *MetricsService) EffectiveSmoothingAlpha() float64 {
	return s.effectiveSmoothingAlpha()
}

func (s *MetricsService) TerminalSource() TerminalSource {
	return s.terminal
}
