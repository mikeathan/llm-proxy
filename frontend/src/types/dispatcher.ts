export interface AutomationRun {
  id: string
  workspace_id?: string
  automation_name: string
  timestamp: string
  output: string
  error: string
  duration_ms: number
  model: string
  recording_ref?: string
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
  recording_ref?: string
  last_output?: string
  last_error?: string
  is_running?: boolean
  history?: AutomationRun[]
}

export interface RecordingMeta {
  id: string
  model: string
  automation_name: string
  timestamp: string
  file_path: string
  file_size: number
  session_id: string
}

export interface RecordingStatus {
  enabled: boolean
  dir: string
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

export type AgentEventType = 'step_start' | 'message' | 'tool_call' | 'tool_result' | 'guardrail_violation' | 'guardrail_blocked' | 'guardrail_invalidated' | 'error' | 'tool_stream' | 'reasoning' | 'lifecycle'

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

export interface AgentGuardrailViolationPayload {
  tool: string
  error: string
}

export interface GuardrailBlockedPayload {
  decision_id: string
  tool: string
  args: string
  reason: string
  category: string
}

export interface GuardrailDecision {
  allow: boolean
  persist: boolean
}

export interface AgentEvent {
  id?: string
  type: AgentEventType
  // channel isolates the event stream producer: 'assistant' (chat) vs
  // 'automation' (scheduled runs). The SSE endpoint serves a single channel
  // per connection so an automation run can never leak into the chat pane.
  channel?: 'assistant' | 'automation'
  conversation_id?: string
  payload: AgentStepStartPayload | AgentMessagePayload | AgentToolCallPayload | AgentToolResultPayload | AgentGuardrailViolationPayload | GuardrailBlockedPayload | string
  timestamp?: string
}

