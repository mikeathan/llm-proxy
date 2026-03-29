package system_metrics

import (
	"fmt"
	"llm-proxy/models"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	procLoadAvgPath = "/proc/loadavg"
	procMemInfoPath = "/proc/meminfo"
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

func (s *MetricsService) Snapshot() MetricsSnapshot {
	host := readHostMetrics(s.nowFn)
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

	if s.throughput != nil {
		if tps, ts := s.throughput.LastTokensPerSecond(); tps > 0 {
			resp.LLMTokensPerSec = tps
			resp.LLMTokensPerSecTS = ts
		}
	}
	return resp
}

func readHostMetrics(now func() time.Time) HostMetrics {
	load1, load5, load15, _ := readLoadAvg()
	mem := readMemInfo()
	cores := runtime.NumCPU()
	pct := loadPct(load1, cores)

	return HostMetrics{
		Load1:          load1,
		Load5:          load5,
		Load15:         load15,
		LoadPct:        pct,
		Cores:          cores,
		MemTotalMB:     mem.totalMB,
		MemFreeMB:      mem.freeMB,
		MemAvailableMB: mem.availMB,
		MemUsedMB:      mem.usedMB,
		Timestamp:      now(),
	}
}

func loadPct(load1 float64, cores int) float64 {
	if cores <= 0 {
		return 0
	}
	return (load1 / float64(cores)) * 100
}

func readLoadAvg() (float64, float64, float64, error) {
	data, err := os.ReadFile(procLoadAvgPath)
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("unexpected loadavg format")
	}
	return parseFloat(fields[0]), parseFloat(fields[1]), parseFloat(fields[2]), nil
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func readMemInfo() memSnapshot {
	data, err := os.ReadFile(procMemInfoPath)
	if err != nil {
		return memSnapshot{}
	}
	lines := strings.Split(string(data), "\n")
	toMB := func(kb float64) float64 { return kb / 1024.0 }
	info := make(map[string]float64)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		info[fields[0]] = parseFloat(fields[1])
	}
	total := toMB(info["MemTotal:"])
	free := toMB(info["MemFree:"])
	avail := toMB(info["MemAvailable:"])
	used := total - avail
	return memSnapshot{
		totalMB: total,
		freeMB:  free,
		availMB: avail,
		usedMB:  used,
	}
}
