import { describe, expect, it } from 'vitest'
import { BUSY_PERCENT, HIGH_PERCENT, MEMORY_THRESHOLDS, loadLevel, summariseHost } from '../../domain/hostStats'
import type { SystemMetrics } from '../../types/metrics'

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
