export interface AutomationRun {
  id: string
  workspace_id?: string
  automation_name: string
  timestamp: string
  output: string
  error: string
  duration_ms: number
  model: string
  events?: AgentEvent[]
}


export interface Automation {
  id: string
  workspace: string
  name: string
  task_file: string
  strategy: string
  trigger: string
  trigger_value?: string
  trigger_type?: string
  model?: string
  last_output?: string
  last_error?: string
  is_running?: boolean
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

export type AgentEventType = 'step_start' | 'message' | 'tool_call' | 'tool_result'

export interface AgentStepStartPayload {
  step: number
}

export interface AgentMessagePayload {
  role: 'assistant' | 'system'
  content: string
}

export interface AgentToolCallPayload {
  function: {
    name: string
    arguments: string
  }
}

export interface AgentToolResultPayload {
  name: string
  result: any
  error?: string
}

export interface AgentEvent {
  type: AgentEventType
  payload: AgentStepStartPayload | AgentMessagePayload | AgentToolCallPayload | AgentToolResultPayload
}

