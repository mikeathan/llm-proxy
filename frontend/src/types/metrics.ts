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

export interface ProcessLogs {
  running: boolean
  name?: string
  ready?: boolean
  started_at?: string
  logs: string
  app_log_ok?: boolean
}
