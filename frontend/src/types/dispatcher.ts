export interface AutomationRun {
  id: string
  workspace_id?: string
  automation_name: string
  timestamp: string
  output: string
  error: string
  duration_ms: number
  model: string
}

export interface Automation {
  id: string
  workspace: string
  name: string
  task_file: string
  strategy: string
  trigger: string
  model?: string
  last_output?: string
  last_error?: string
  history?: AutomationRun[]
}

export interface AgentState {
  last_output: string
  last_error: string
  next_run_at: string
  is_running: boolean
  last_pulse: string
  history: AutomationRun[]
  last_runs: Record<string, AutomationRun>
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
