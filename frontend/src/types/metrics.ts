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

// What the service and its model server cost the machine (GET /admin/api/metrics/footprint). Optional fields are absent
// when the platform does not report them (open files and per-process GPU memory on macOS); cpu_percent is null until a second reading.
export interface ProcessFootprint {
  role: 'service' | 'model'
  pid: number
  cpu_percent: number | null
  rss_mb: number
  threads: number
  open_files?: number
  uptime_seconds: number
  gpu_vram_mb?: number
  gpu_gtt_mb?: number
}

export interface ServiceFootprint {
  processes: ProcessFootprint[]
  go: { heap_mb: number; total_mb: number; goroutines: number; gc_cycles: number }
  timestamp: string
}

/** One process of the footprint reduced to display strings. */
export interface FootprintRow {
  key: string
  title: string
  cpu: string
  memory: string
  gpu: string
  threads: string
  openFiles: string | null
  uptime: string
}

export interface FootprintSummary {
  rows: FootprintRow[]
  go: { heap: string; total: string; goroutines: string; gcCycles: string }
}

// One metrics sample kept for the Overview sparklines (plan V8: the backend
// has no history, so the client keeps a short one "since page load").
export interface MetricsSample {
  tokensPerSecond: number
  loadPercent: number
}

export type LoadLevel = 'ok' | 'busy' | 'high'

/** The colour a meter takes: a load level mapped through `loadMeterState` (domain/hostStats). */
export type MeterState = 'info' | 'running' | 'error'

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
