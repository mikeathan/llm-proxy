import { describe, expect, it } from 'vitest'
import { PRODUCT_NAME } from '../../config/brand'
import { BUSY_PERCENT, HIGH_PERCENT, MEMORY_THRESHOLDS, VRAM_THRESHOLDS, loadLevel, loadMeterState, summariseFootprint, summariseHost } from '../../domain/hostStats'
import type { ServiceFootprint, SystemMetrics } from '../../types/metrics'

const GPU = {
  name: 'Apple GPU',
  vendor: 'apple',
  memory_used_mb: 16281.6,
  memory_total_mb: 16384,
  memory_utilization_percent: 99.4,
  utilization_percent: 4.9,
  temperature_c: 37,
}
const BASE: SystemMetrics = { load_percent: 0.2, mem_used_mb: 7475.2, mem_total_mb: 47923.2, llm_tokens_per_sec: 33.2, gpu: GPU }

describe('loadLevel', () => {
  it.each([
    [0, 'ok'],
    [BUSY_PERCENT - 0.1, 'ok'],
    [BUSY_PERCENT, 'busy'],
    [HIGH_PERCENT - 0.1, 'busy'],
    [HIGH_PERCENT, 'high'],
    [100, 'high'],
  ])('%d%% is %s', (percent, level) => {
    expect(loadLevel(percent)).toBe(level)
  })
})

// The header figure and every meter colour a load from the same level, so one reading never looks different in two places.
describe('loadMeterState', () => {
  it.each([
    [10, 'info'],
    [BUSY_PERCENT, 'running'],
    [82.7, 'error'],
  ])('%d%% load is %s', (percent, state) => {
    expect(loadMeterState(percent)).toBe(state)
  })
  it('never alarms on full VRAM, which a loaded model fills on purpose', () => {
    expect(loadMeterState(50, VRAM_THRESHOLDS)).toBe('info')
    expect(loadMeterState(75, VRAM_THRESHOLDS)).toBe('running')
    expect(loadMeterState(100, VRAM_THRESHOLDS)).toBe('running')
  })
})

describe('loadLevel for memory', () => {
  // Used memory includes the OS cache, so ~70% is normal and must not look like a warning.
  it.each([
    [72, 'ok'],
    [MEMORY_THRESHOLDS.busy, 'busy'],
    [MEMORY_THRESHOLDS.high - 0.1, 'busy'],
    [MEMORY_THRESHOLDS.high, 'high'],
  ])('%d%% memory is %s', (percent, level) => {
    expect(loadLevel(percent, MEMORY_THRESHOLDS)).toBe(level)
  })
})

describe('summariseHost', () => {
  it('derives CPU, memory, GPU and throughput from one metrics sample', () => {
    const s = summariseHost(BASE)
    expect(s.cpuPercent).toBe(0.2)
    expect(s.memory).toEqual({ percent: 16, text: '7.3 / 46.8 GB' })
    expect(s.gpu).toEqual({ name: 'Apple GPU', corePercent: 4.9, vramPercent: 99, vramText: '15.9 / 16.0 GB', temperatureC: 37 })
    expect(s.gpuError).toBeNull()
    expect(s.tokensPerSecond).toBe(33.2)
  })

  it('treats missing optional figures as zero', () => {
    const s = summariseHost({ mem_used_mb: 1, mem_total_mb: 2 })
    expect(s.cpuPercent).toBe(0)
    expect(s.tokensPerSecond).toBe(0)
    expect(s.gpu).toBeNull()
  })

  it('reports no GPU with the backend reason, and never divides by a zero total', () => {
    const s = summariseHost({ mem_used_mb: 5, mem_total_mb: 0, gpu_error: 'rocm-smi not found' })
    expect(s.memory.percent).toBe(0)
    expect(s.gpu).toBeNull()
    expect(s.gpuError).toBe('rocm-smi not found')
  })

  it('names the GPU by vendor when it has no name, and leaves an unreported temperature null', () => {
    const { name: _name, ...unnamed } = GPU
    const s = summariseHost({ ...BASE, gpu: { ...unnamed, temperature_c: Number.NaN } })
    expect(s.gpu?.name).toBe('apple')
    expect(s.gpu?.temperatureC).toBeNull()
  })
})

const FOOTPRINT: ServiceFootprint = {
  processes: [
    { role: 'service', pid: 10, cpu_percent: 3.14, rss_mb: 512, threads: 21, open_files: 34, uptime_seconds: 3725 },
    { role: 'model', pid: 20, cpu_percent: null, rss_mb: 20480, threads: 40, uptime_seconds: 90, gpu_vram_mb: 18432, gpu_gtt_mb: 2048 },
  ],
  go: { heap_mb: 48.2, total_mb: 90.7, goroutines: 120, gc_cycles: 33 },
  timestamp: '2026-11-27T10:00:00Z',
}

describe('summariseFootprint', () => {
  const view = summariseFootprint(FOOTPRINT, 40960, 'Qwen3.6 35B A3B')

  it('names the service and the running model', () => {
    expect(view.rows.map((r) => r.title)).toEqual([`${PRODUCT_NAME} service`, 'Qwen3.6 35B A3B'])
  })
  it('reads CPU as a share of one core, and says "Measuring" until a second reading gives a delta', () => {
    expect(view.rows[0]?.cpu).toBe('3.1%')
    expect(view.rows[1]?.cpu).toBe('Measuring…')
  })
  it('shows resident memory with its share of the machine', () => {
    expect(view.rows[0]?.memory).toBe('512 MB · 1.3% of RAM')
    expect(view.rows[1]?.memory).toBe('20.0 GB · 50.0% of RAM')
  })
  it('shows GPU memory when reported (with the shared part) and "Not reported" when not', () => {
    expect(view.rows[1]?.gpu).toBe('18.0 GB + 2.0 GB shared')
    expect(view.rows[0]?.gpu).toBe('Not reported')
  })
  it('leaves open files out when the platform does not report them', () => {
    expect(view.rows[0]?.openFiles).toBe('34')
    expect(view.rows[1]?.openFiles).toBeNull()
  })
  it('summarises the Go runtime', () => {
    expect(view.go).toEqual({ heap: '48 MB', total: '91 MB', goroutines: '120', gcCycles: '33' })
  })
  it('falls back to a generic title when the model server has no known name', () => {
    expect(summariseFootprint(FOOTPRINT, 40960).rows[1]?.title).toBe('Model server')
  })
})
