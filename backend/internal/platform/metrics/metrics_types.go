package metrics

import (
	"context"
	"os"
	"sync"
	"time"
)

type HostMetrics struct {
	Load1          float64   `json:"load1"`
	Load5          float64   `json:"load5"`
	Load15         float64   `json:"load15"`
	LoadPct        float64   `json:"load_percent"`
	Cores          int       `json:"cores"`
	MemTotalMB     float64   `json:"mem_total_mb"`
	MemFreeMB      float64   `json:"mem_free_mb"`
	MemAvailableMB float64   `json:"mem_available_mb"`
	MemUsedMB      float64   `json:"mem_used_mb"`
	Timestamp      time.Time `json:"timestamp"`
}

type GPUMetrics struct {
	Vendor               string  `json:"vendor"`
	Name                 string  `json:"name,omitempty"`
	UtilizationPct       float64 `json:"utilization_percent"`
	MemoryUtilizationPct float64 `json:"memory_utilization_percent"`
	MemoryTotalMB        float64 `json:"memory_total_mb,omitempty"`
	MemoryUsedMB         float64 `json:"memory_used_mb,omitempty"`
	GttUsedMB            float64 `json:"gtt_used_mb,omitempty"`
	TemperatureC         float64 `json:"temperature_c,omitempty"`
}

type MetricsSnapshot struct {
	HostMetrics
	GPU               *GPUMetrics `json:"gpu,omitempty"`
	GPUProvider       string      `json:"gpu_provider,omitempty"`
	GPUError          string      `json:"gpu_error,omitempty"`
	GPUCorePercent    float64     `json:"gpu_core_percent"`
	GPUMemoryPercent  float64     `json:"gpu_memory_percent"`
	GPUMemoryUsedMB   float64     `json:"gpu_memory_used_mb"`
	GPUMemoryTotalMB  float64     `json:"gpu_memory_total_mb"`
	LLMTokensPerSec   float64     `json:"llm_tokens_per_sec,omitempty"`
	LLMTokensPerSecTS time.Time   `json:"llm_tokens_per_sec_ts,omitempty"`
	IdleSandboxes     int         `json:"idle_sandboxes"`
	ActiveSandboxes   int         `json:"active_sandboxes"`
}

type GPUProvider interface {
	Name() string
	Sample() (*GPUMetrics, error)
}

type ThroughputSource interface {
	LastTokensPerSecond() (float64, time.Time)
}

type SandboxSource interface {
	HealthCheck() (idle, active int)
}

type MetricsService struct {
	gpu             GPUProvider
	gpuProviderName string
	gpuInitErr      string
	throughput      ThroughputSource
	sandbox         SandboxSource
	nowFn           func() time.Time
}

type memSnapshot struct {
	totalMB float64
	freeMB  float64
	availMB float64
	usedMB  float64
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type nvidiaSMIProvider struct {
	binary string
	index  int
	run    commandRunner
}

type rocmSMIProvider struct {
	binary string
	index  int
	run    commandRunner
}

type sysfsProvider struct {
	basePath string
	files    map[string]*os.File
}

type amdGpuTopProvider struct {
	binary string
	index  int
	run    commandRunner
}

// TokenTracker watches llama-server output for tokens/sec lines.
// It keeps the most recent numeric throughput and when it was seen.
type TokenTracker struct {
	mu     sync.Mutex
	buf    []byte
	last   float64
	lastAt time.Time
}
