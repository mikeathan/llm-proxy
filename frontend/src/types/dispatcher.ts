export interface Automation {
  id: string
  workspace: string
  name: string
  task_file: string
  strategy: string
  trigger: string
}

export interface DispatcherMetrics {
  total_executions: number
  successful: number
  failed: number
  skipped: number
  total_latency_ms: number
}

export interface TriggerResponse {
  status: string
  workspace: string
  automation: string
}
