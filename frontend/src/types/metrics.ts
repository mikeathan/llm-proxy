// System metrics and logging types
export interface GpuInfo {
  name?: string
  vendor?: string
  memory_used_mb: number
  memory_total_mb: number
  memory_utilization_percent: number
  utilization_percent: number
  temperature_c: number
}

export interface SystemMetrics {
  load_percent?: number
  mem_used_mb: number
  mem_total_mb: number
  gpu?: GpuInfo
  gpu_error?: string
  llm_tokens_per_sec?: number
}

// One metrics sample kept for the Overview sparklines (plan V8: the backend
// has no history, so the client keeps a short one "since page load").
export interface MetricsSample {
  tokensPerSecond: number
  loadPercent: number
}

export type LoadLevel = 'ok' | 'busy' | 'high'

// One metrics sample reduced to what the header strip and Overview show.
export interface HostSummary {
  cpuPercent: number
  memory: { percent: number; text: string }
  gpu: {
    name: string
    corePercent: number
    vramPercent: number
    vramText: string
    temperatureC: number | null
  } | null
  gpuError: string | null
  tokensPerSecond: number
}

export interface ProcessLogs {
  running: boolean
  name?: string
  ready?: boolean
  started_at?: string
  logs: string
  app_log_ok?: boolean
}
