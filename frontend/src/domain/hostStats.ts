import type { GpuInfo, HostSummary, LoadLevel, MeterState, SystemMetrics } from '../types/metrics'
import { formatMemoryUsage } from '../utils/format/units'

// Pure view-model of one /admin/api/metrics sample, shared by the header strip
// and the Overview so the two can never disagree about a figure.

export const BUSY_PERCENT = 50
export const HIGH_PERCENT = 80
// Used memory includes the OS cache, so the same load means less: a box at 70%
// is normal, not a warning.
export const MEMORY_THRESHOLDS = { busy: 75, high: 90 } as const
// A model that is loaded fills VRAM on purpose, so full VRAM is busy but never an alarm.
export const VRAM_THRESHOLDS = { busy: 75, high: Infinity } as const
const DEFAULT_THRESHOLDS = { busy: BUSY_PERCENT, high: HIGH_PERCENT }
const PERCENT = 100

/** The colour a meter takes for a load level: one rule for the header figures and every meter. */
const METER_STATE: Record<LoadLevel, MeterState> = { ok: 'info', busy: 'running', high: 'error' }
export const loadMeterState = (percent: number, thresholds: { busy: number; high: number } = DEFAULT_THRESHOLDS): MeterState =>
  METER_STATE[loadLevel(percent, thresholds)]

/** How loaded a resource is: drives the colour (never the only signal — the number is always shown). */
export function loadLevel(percent: number, thresholds: { busy: number; high: number } = DEFAULT_THRESHOLDS): LoadLevel {
  if (percent >= thresholds.high) return 'high'
  if (percent >= thresholds.busy) return 'busy'
  return 'ok'
}

const percentOf = (used: number, total: number): number => (total ? Math.round((used / total) * PERCENT) : 0)

function summariseGpu(g: GpuInfo): NonNullable<HostSummary['gpu']> {
  return {
    name: g.name || g.vendor || 'GPU',
    corePercent: g.utilization_percent,
    vramPercent: percentOf(g.memory_used_mb, g.memory_total_mb),
    vramText: formatMemoryUsage(g.memory_used_mb, g.memory_total_mb),
    // Some GPUs (e.g. Apple silicon) report no temperature.
    temperatureC: Number.isFinite(g.temperature_c) ? g.temperature_c : null,
  }
}

export function summariseHost(metrics: SystemMetrics): HostSummary {
  return {
    cpuPercent: metrics.load_percent ?? 0,
    memory: {
      percent: percentOf(metrics.mem_used_mb, metrics.mem_total_mb),
      text: formatMemoryUsage(metrics.mem_used_mb, metrics.mem_total_mb),
    },
    gpu: metrics.gpu ? summariseGpu(metrics.gpu) : null,
    gpuError: metrics.gpu_error ?? null,
    tokensPerSecond: metrics.llm_tokens_per_sec ?? 0,
  }
}
