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
	return &MetricsService{
		gpu:             provider,
		gpuProviderName: name,
		gpuInitErr:      initErr,
		nowFn:           time.Now,
	}
}

func (s *MetricsService) SetThroughputSource(src ThroughputSource) {
	s.throughput = src
}

func (s *MetricsService) SetSandboxSource(src SandboxSource) {
	s.sandbox = src
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

	gpu, err := s.gpu.Sample()
	if err != nil {
		resp.GPUError = err.Error()
		return resp
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

	if s.sandbox != nil {
		idle, active := s.sandbox.HealthCheck()
		resp.IdleSandboxes = idle
		resp.ActiveSandboxes = active
	}
	return resp
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
