package metrics

import (
	"testing"
	"time"
)

type fakeGPUProvider struct {
	sample *GPUMetrics
	err    error
	calls  int
}

func (f *fakeGPUProvider) Name() string {
	return "fake"
}

func (f *fakeGPUProvider) Sample() (*GPUMetrics, error) {
	f.calls++
	return f.sample, f.err
}

type fakeThroughput struct {
	tps float64
	ts  time.Time
}

func (f *fakeThroughput) LastTokensPerSecond() (float64, time.Time) {
	return f.tps, f.ts
}

func TestMetricsService_SmoothsGPUUtilizationSpikes(t *testing.T) {
	provider := &scriptedGPUProvider{values: []float64{9, 0, 23, 0}}
	svc := &MetricsService{
		gpu:               provider,
		gpuProviderName:   "scripted",
		nowFn:             time.Now,
		gpuSmoothingAlpha: 0.3,
	}

	first := svc.Snapshot()
	if first.GPUCorePercent != 9 {
		t.Fatalf("expected seed value 9, got %v", first.GPUCorePercent)
	}

	svc.Snapshot()
	third := svc.Snapshot()
	if third.GPUCorePercent >= 23 {
		t.Fatalf("expected core spike dampened below 23, got %v", third.GPUCorePercent)
	}
	if third.GPUCorePercent <= 0 {
		t.Fatalf("expected dampened core value above 0, got %v", third.GPUCorePercent)
	}
	if third.GPUMemoryPercent >= 23 {
		t.Fatalf("expected memory spike dampened below 23, got %v", third.GPUMemoryPercent)
	}
}

func TestMetricsService_SetSmoothingAlpha(t *testing.T) {
	svc := &MetricsService{
		gpu:               &fakeGPUProvider{sample: &GPUMetrics{Vendor: "fake"}},
		gpuProviderName:   "fake",
		nowFn:             time.Now,
		gpuSmoothingAlpha: 0.3,
	}

	svc.SetSmoothingAlpha(0)
	if got := svc.effectiveSmoothingAlpha(); got != defaultGPUSmoothingAlpha {
		t.Fatalf("expected default alpha after 0, got %v", got)
	}
	svc.SetSmoothingAlpha(2)
	if got := svc.effectiveSmoothingAlpha(); got != defaultGPUSmoothingAlpha {
		t.Fatalf("expected default alpha after >1, got %v", got)
	}

	svc.SetSmoothingAlpha(0.9)
	if got := svc.effectiveSmoothingAlpha(); got != 0.9 {
		t.Fatalf("expected 0.9 after SetSmoothingAlpha, got %v", got)
	}
}

func TestMetricsService_SeedsFromMeanOfFirstSamples(t *testing.T) {
	provider := &scriptedGPUProvider{values: []float64{30, 0, 0, 0, 0, 0, 0, 0, 0}}
	svc := &MetricsService{
		gpu:               provider,
		gpuProviderName:   "scripted",
		nowFn:             time.Now,
		gpuSmoothingAlpha: 0.3,
	}

	svc.Snapshot()
	svc.Snapshot()
	third := svc.Snapshot()
	if third.GPUCorePercent != 10 {
		t.Fatalf("expected mean of first 3 samples (30,0,0) = 10, got %v", third.GPUCorePercent)
	}
	if third.GPUMemoryPercent != 10 {
		t.Fatalf("expected memory seed mean 10, got %v", third.GPUMemoryPercent)
	}
}

type scriptedGPUProvider struct {
	values []float64
	idx    int
}

func (p *scriptedGPUProvider) Name() string { return "scripted" }

func (p *scriptedGPUProvider) Sample() (*GPUMetrics, error) {
	v := 0.0
	if p.idx < len(p.values) {
		v = p.values[p.idx]
		p.idx++
	}
	return &GPUMetrics{Vendor: "scripted", UtilizationPct: v, MemoryUtilizationPct: v}, nil
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

func TestReadHostMetrics_Integration(t *testing.T) {
	svc := &MetricsService{
		nowFn: time.Now,
	}
	m := svc.readHostMetrics()

	if m.Cores <= 0 {
		t.Errorf("expected Cores > 0, got %d", m.Cores)
	}
	if m.MemTotalMB <= 0 {
		t.Errorf("expected MemTotalMB > 0, got %f", m.MemTotalMB)
	}
	// On most systems Load1 will be >= 0
	if m.Load1 < 0 {
		t.Errorf("expected Load1 >= 0, got %f", m.Load1)
	}
}

func TestMetricsServiceSnapshot_UsesCacheWhenAvailable(t *testing.T) {
	cached := &GPUMetrics{Vendor: "cached", UtilizationPct: 75, MemoryUsedMB: 512, MemoryTotalMB: 1024, MemoryUtilizationPct: 50}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	provider := &fakeGPUProvider{sample: &GPUMetrics{Vendor: "fresh", UtilizationPct: 10}}

	svc := &MetricsService{
		gpu:             provider,
		gpuProviderName: "fake",
		nowFn:           func() time.Time { return now },
		gpuCached:       cached,
		gpuCachedAt:     now,
	}

	snap := svc.Snapshot()

	if snap.GPUCorePercent != 75 {
		t.Fatalf("expected cached core percent 75, got %v", snap.GPUCorePercent)
	}
	if snap.GPU.Vendor != "cached" {
		t.Fatalf("expected cached vendor 'cached', got %v", snap.GPU.Vendor)
	}
	if provider.calls > 0 {
		t.Fatalf("expected no Sample calls when cache is available, got %d", provider.calls)
	}
}

func TestMetricsServiceSnapshot_FallbackWhenCacheEmpty(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	provider := &fakeGPUProvider{sample: &GPUMetrics{Vendor: "live", UtilizationPct: 33}}

	svc := &MetricsService{
		gpu:             provider,
		gpuProviderName: "fake",
		nowFn:           func() time.Time { return now },
	}

	snap := svc.Snapshot()

	if snap.GPUCorePercent != 33 {
		t.Fatalf("expected live core percent 33, got %v", snap.GPUCorePercent)
	}
	if snap.GPU.Vendor != "live" {
		t.Fatalf("expected live vendor 'live', got %v", snap.GPU.Vendor)
	}
	if provider.calls != 1 {
		t.Fatalf("expected 1 Sample call for fallback, got %d", provider.calls)
	}
}

func TestMetricsServiceSnapshot_CachedError(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	cached := &GPUMetrics{Vendor: "stale", UtilizationPct: 80}
	provider := &fakeGPUProvider{sample: &GPUMetrics{Vendor: "never-called", UtilizationPct: 0}}

	svc := &MetricsService{
		gpu:             provider,
		gpuProviderName: "fake",
		nowFn:           func() time.Time { return now },
		gpuCached:       cached,
		gpuCachedErr:    nil,
		gpuCachedAt:     now,
	}

	snap := svc.Snapshot()
	if snap.GPUError != "" {
		t.Fatalf("expected no GPUError with good cached data, got %s", snap.GPUError)
	}
	if snap.GPUCorePercent != 80 {
		t.Fatalf("expected cached core percent 80, got %v", snap.GPUCorePercent)
	}
	if provider.calls > 0 {
		t.Fatalf("expected no Sample calls when cache available, got %d", provider.calls)
	}
}

func TestMetricsService_Start(t *testing.T) {
	provider := &fakeGPUProvider{sample: &GPUMetrics{Vendor: "bg", UtilizationPct: 42}}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	svc := &MetricsService{
		gpu:               provider,
		gpuProviderName:   "fake",
		nowFn:             func() time.Time { return now },
		gpuSampleInterval: 100 * time.Millisecond,
	}
	svc.Start()

	svc.gpuMu.RLock()
	hasCache := !svc.gpuCachedAt.IsZero()
	cachedUtil := 0.0
	if svc.gpuCached != nil {
		cachedUtil = svc.gpuCached.UtilizationPct
	}
	svc.gpuMu.RUnlock()

	if !hasCache {
		t.Fatal("expected cache to be populated after Start() sync sample")
	}
	if cachedUtil != 42 {
		t.Fatalf("expected cached utilization 42, got %v", cachedUtil)
	}
	if provider.calls != 1 {
		t.Fatalf("expected 1 Sample call from Start sync sample, got %d", provider.calls)
	}

	svc.Stop()
}

func TestMetricsService_StartStopNoLeak(t *testing.T) {
	provider := &fakeGPUProvider{sample: &GPUMetrics{Vendor: "bg", UtilizationPct: 10}}
	svc := &MetricsService{
		gpu:               provider,
		gpuProviderName:   "fake",
		nowFn:             time.Now,
		gpuSampleInterval: 10 * time.Millisecond,
	}
	svc.Start()
	svc.Stop()

	// Repeated Stop must not panic
	svc.Stop()
}

func TestMetricsService_StartNoProvider(t *testing.T) {
	svc := &MetricsService{
		gpu:               nil,
		gpuProviderName:   "none",
		nowFn:             time.Now,
		gpuSampleInterval: 10 * time.Second,
	}
	svc.Start()

	svc.gpuMu.RLock()
	hasCache := !svc.gpuCachedAt.IsZero()
	svc.gpuMu.RUnlock()

	if hasCache {
		t.Fatal("expected no cache when provider is nil")
	}
}

type countingGPUProvider struct {
	calls int
}

func (c *countingGPUProvider) Name() string { return "counting" }

func (c *countingGPUProvider) Sample() (*GPUMetrics, error) {
	c.calls++
	return &GPUMetrics{Vendor: "counting", UtilizationPct: float64(c.calls)}, nil
}

func TestMetricsService_StartTickerUpdatesCache(t *testing.T) {
	provider := &countingGPUProvider{}
	svc := &MetricsService{
		gpu:               provider,
		gpuProviderName:   "counting",
		nowFn:             time.Now,
		gpuSampleInterval: 20 * time.Millisecond,
	}
	svc.Start()

	svc.gpuMu.RLock()
	firstCached := svc.gpuCached
	svc.gpuMu.RUnlock()
	if firstCached == nil || firstCached.UtilizationPct != 1 {
		t.Fatalf("expected sync sample utilization 1, got %v", firstCached)
	}

	time.Sleep(150 * time.Millisecond)

	svc.gpuMu.RLock()
	latestCached := svc.gpuCached
	svc.gpuMu.RUnlock()
	svc.Stop()

	if provider.calls < 3 {
		t.Fatalf("expected background ticker to sample >=3 times, got %d", provider.calls)
	}
	if latestCached == nil || latestCached.UtilizationPct <= firstCached.UtilizationPct {
		t.Fatalf("expected cache refreshed to a later sample, first=%v latest=%v", firstCached, latestCached)
	}
}

func TestMetricsService_SnapshotStaleness(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	var frozen time.Time
	nowFn := func() time.Time { return frozen }

	cached := &GPUMetrics{Vendor: "cached", UtilizationPct: 50}

	// Stale: cache older than 2x interval
	frozen = now
	svcStale := &MetricsService{
		gpu:               &fakeGPUProvider{sample: &GPUMetrics{Vendor: "live"}},
		gpuProviderName:   "fake",
		nowFn:             nowFn,
		gpuSampleInterval: 10 * time.Second,
		gpuCached:         cached,
		gpuCachedAt:       now.Add(-25 * time.Second),
	}
	snapStale := svcStale.Snapshot()
	if !snapStale.GPUStale {
		t.Fatal("expected GPUStale true for cache older than 2x interval")
	}
	if snapStale.GPUCacheAgeSec < 25 {
		t.Fatalf("expected cache age >= 25s, got %v", snapStale.GPUCacheAgeSec)
	}

	// Fresh: cache within interval
	frozen = now
	svcFresh := &MetricsService{
		gpu:               &fakeGPUProvider{sample: &GPUMetrics{Vendor: "live"}},
		gpuProviderName:   "fake",
		nowFn:             nowFn,
		gpuSampleInterval: 10 * time.Second,
		gpuCached:         cached,
		gpuCachedAt:       now.Add(-3 * time.Second),
	}
	snapFresh := svcFresh.Snapshot()
	if snapFresh.GPUStale {
		t.Fatal("expected GPUStale false for fresh cache")
	}
	if snapFresh.GPUCacheAgeSec < 3 {
		t.Fatalf("expected cache age >= 3s, got %v", snapFresh.GPUCacheAgeSec)
	}

	// No cache: staleness fields unset
	frozen = now
	svcEmpty := &MetricsService{
		gpu:               &fakeGPUProvider{sample: cached},
		gpuProviderName:   "fake",
		nowFn:             nowFn,
		gpuSampleInterval: 10 * time.Second,
	}
	snapEmpty := svcEmpty.Snapshot()
	if snapEmpty.GPUStale {
		t.Fatal("expected GPUStale false when no cache present")
	}
	if snapEmpty.GPUCacheAgeSec != 0 {
		t.Fatalf("expected zero cache age when no cache, got %v", snapEmpty.GPUCacheAgeSec)
	}
}
