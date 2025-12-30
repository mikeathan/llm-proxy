package system_metrics

import (
	"testing"
	"time"
)

type fakeGPUProvider struct {
	sample *GPUMetrics
	err    error
}

func (f *fakeGPUProvider) Name() string {
	return "fake"
}

func (f *fakeGPUProvider) Sample() (*GPUMetrics, error) {
	return f.sample, f.err
}

type fakeThroughput struct {
	tps float64
	ts  time.Time
}

func (f *fakeThroughput) LastTokensPerSecond() (float64, time.Time) {
	return f.tps, f.ts
}

func TestMetricsServiceSnapshot_WithGPUAndThroughput(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	provider := &fakeGPUProvider{sample: &GPUMetrics{
		Vendor:               "amd",
		Name:                 "card0",
		UtilizationPct:       42,
		MemoryUsedMB:         100,
		MemoryTotalMB:        200,
		MemoryUtilizationPct: 50,
		TemperatureC:         60,
	}}

	svc := &MetricsService{
		gpu:             provider,
		gpuProviderName: "fake",
		nowFn:           func() time.Time { return now },
	}
	svc.SetThroughputSource(&fakeThroughput{tps: 12.5, ts: now.Add(-time.Second)})

	snap := svc.Snapshot()

	if snap.GPUError != "" {
		t.Fatalf("unexpected GPUError: %s", snap.GPUError)
	}
	if snap.GPU == nil || snap.GPU.Name != "card0" {
		t.Fatalf("expected GPU snapshot")
	}
	if snap.GPUCorePercent != 42 {
		t.Fatalf("unexpected GPUCorePercent: %v", snap.GPUCorePercent)
	}
	if snap.GPUMemoryUsedMB != 100 || snap.GPUMemoryTotalMB != 200 {
		t.Fatalf("unexpected memory totals")
	}
	if snap.LLMTokensPerSec != 12.5 {
		t.Fatalf("unexpected tokens/sec: %v", snap.LLMTokensPerSec)
	}
	if snap.Timestamp != now {
		t.Fatalf("unexpected timestamp: %v", snap.Timestamp)
	}
}

func TestMetricsServiceSnapshot_GPUInitError(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	svc := &MetricsService{
		gpu:             nil,
		gpuProviderName: "sysfs",
		gpuInitErr:      "sysfs path /nope not readable",
		nowFn:           func() time.Time { return now },
	}

	snap := svc.Snapshot()
	if snap.GPUError == "" {
		t.Fatalf("expected GPUError")
	}
	if snap.GPU != nil {
		t.Fatalf("expected nil GPU snapshot on init error")
	}
}

func TestMetricsServiceSnapshot_IgnoresZeroThroughput(t *testing.T) {
	svc := &MetricsService{
		gpu:             nil,
		gpuProviderName: "disabled",
		nowFn:           time.Now,
	}
	svc.SetThroughputSource(&fakeThroughput{tps: 0, ts: time.Now()})

	snap := svc.Snapshot()
	if snap.LLMTokensPerSec != 0 {
		t.Fatalf("expected tokens/sec to remain zero")
	}
	if !snap.LLMTokensPerSecTS.IsZero() {
		t.Fatalf("expected tokens/sec timestamp to be zero")
	}
}
