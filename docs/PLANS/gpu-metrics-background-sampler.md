# GPU Metrics Background Sampler

**Status:** approved — implemented (2026-07-23)
**Date:** 2026-07-23

## Problem

The frontend "Host Resources" panel polls `GET /admin/api/metrics` every 5s. On macOS, each call runs `ioreg -c IOAccelerator -r -l` (a slow system registry scan, 500ms–1s). On Linux/NVIDIA, each call spawns a fresh `nvidia-smi` subprocess. This per-request GPU sampling keeps the frontend polling the backend, and each poll triggers a Vue reactivity → DOM diff → GPU compositing cycle — contributing to 70–80% GPU core utilization shown in the Host Resources panel.

**Root cause:** GPU sampling is done synchronously inside `Snapshot()`, coupling the HTTP response latency to the GPU provider's runtime. The expensive operation runs on every frontend poll, and the frontend polls at 5s.

## Solution

**Decouple sampling from serving.** A background goroutine runs `ioreg`/`nvidia-smi` at a configurable interval (default 10s) and caches the result. `GET /admin/api/metrics` reads the cached value — zero subprocess overhead per HTTP request. The frontend still gets fresh readings every query, but the backend I/O drops from 12+ `ioreg` calls/minute to 6.

**On the frontend:** Bump poll interval to 10s (matches backend cadence). Split the `useMetrics` composable so components that only need `logLevel` (`Logs.vue`, `Settings.vue`) don't trigger metrics polling.

## Changes

### Backend — metrics_types.go
- Add `stopCh`, `gpuMu` (RWMutex), `gpuCached *GPUMetrics`, `gpuCachedErr error`, `gpuCachedAt time.Time` fields to `MetricsService`.

### Backend — metrics_service.go
- `start()`: If a GPU provider is available, take one synchronous sample for immediate data, then launch a goroutine with `time.NewTicker(gpuSampleInterval)` that calls `s.gpu.Sample()` and stores the result under mutex.
- `Snapshot()`: Read cached GPU value under `RLock`. If cache is empty (before first sample completes), fall back to synchronous sample.
- `Stop()`: Close `stopCh` to terminate the sampler goroutine.

### Backend — models/config.go
- Add `GPUSampleInterval int` to `MetricsConfig` (default 10s, 0/negative = no background sampling, fall back to legacy synchronous).

### Backend — app_context.go
- `refreshMetricsService()`: Stop old `MetricsService` before creating new one (so GPU config changes restart the sampler).
- `AppContext.Shutdown()`: Stop the metrics service.

### Frontend — constants/api.ts
- `POLL_INTERVAL_MS` 5000 → 10000.

### Frontend — composables/system/useMetrics.ts
- Split into `useMetrics()` (metrics polling) and `useLogLevel()` (log-level only, no polling).

### Frontend — components
- `Logs.vue` and `Settings.vue`: Use `useLogLevel()` instead of `useMetrics()`.
