// Global configuration types and automation read models.
import type { LoopStrategy } from './model'

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
  run_dir_name?: string
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
  loop_strategy?: LoopStrategy
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

export type AgentEventType = 'step_start' | 'message' | 'tool_call' | 'tool_result' | 'guardrail_violation' | 'guardrail_blocked' | 'guardrail_invalidated' | 'error' | 'tool_stream' | 'reasoning' | 'lifecycle' | 'upstream'

// Lifecycle phase vocabulary (SSOT). These string VALUES are sent verbatim by
// the Go backend over SSE and MUST stay in sync with the Phase* constants in
// backend/internal/core/assistant/agent_events.go — never rename a value here,
// or you break the wire. Prefer LIFECYCLE_PHASES.x over bare string literals.
export const LIFECYCLE_PHASES = {
  agentThinking: 'agent_thinking',
  stillThinking: 'still_thinking',
  sessionStarted: 'session_started',
  sessionProgress: 'session_progress',
  sessionCompleted: 'session_completed',
  completed: 'completed',
} as const

export type LifecyclePhase = (typeof LIFECYCLE_PHASES)[keyof typeof LIFECYCLE_PHASES]

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

// UpstreamEventPayload describes a transient upstream LLM failure that is being
// retried. Emitted on the 'upstream' AgentEventType. MUST stay in sync with
// UpstreamEventPayload in backend/internal/core/assistant/agent_events.go.
export interface UpstreamEventPayload {
  event: string // "retry"
  reason: 'transport' | 'status'
  attempt: number // 1-based attempt being retried
  max_attempts: number
  error?: string // transport error text
  err_class?: string // transport error bucket ("connection-closed", "timeout", "tls", ...)
  status?: number // upstream HTTP status
  elapsed_ms?: number
}

export interface AgentEvent {
  id?: string
  type: AgentEventType
  // channel isolates the event stream producer: 'assistant' (chat) vs
  // 'automation' (scheduled runs). The SSE endpoint serves a single channel
  // per connection so an automation run can never leak into the chat pane.
  channel?: 'assistant' | 'automation'
  conversation_id?: string
  payload: AgentStepStartPayload | AgentMessagePayload | AgentToolCallPayload | AgentToolResultPayload | AgentGuardrailViolationPayload | GuardrailBlockedPayload | UpstreamEventPayload | string
  timestamp?: string
}

