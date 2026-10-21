// Global configuration types and automation read models.
import type { LoopStrategy } from './model'
import type { NetworkGrant } from './admin'
import type { MemoryMode, NotifyConfig } from './automation'

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
  // Workspace path of the run's saved final report; absent when none was saved.
  report_path?: string
  events?: AgentEvent[]
  // Non-fatal problems, e.g. a report that could not be delivered; the run still completed.
  warnings?: string[]
}

// What a finished run's tag says: failed beats warnings, warnings beat a plain completion.
export type RunOutcome = 'failed' | 'warning' | 'completed'


export interface Automation {
  id: string
  workspace: string
  name: string
  task_file: string
  strategy: string
  trigger: string
  trigger_value?: string
  trigger_type?: string
  // When the scheduler will next fire it (RFC 3339, the server's clock); absent for a manual automation.
  next_run_at?: string
  model?: string
  loop_strategy?: LoopStrategy
  recording_ref?: string
  last_output?: string
  last_error?: string
  last_warnings?: string[]
  // Workspace path of the latest run's saved final report.
  last_report_path?: string
  is_running?: boolean
  // Queued in the run scheduler waiting for a lane slot; queue_position is
  // 1-based. Both are empty while the run executes or sits idle. Serialized by
  // the backend as "queued"/"queue_position" (AutomationInfo).
  queued?: boolean
  queue_position?: number
  history?: AutomationRun[]
  network_grant?: NetworkGrant
  memory_mode?: MemoryMode
  notify?: NotifyConfig
  skip_if_busy?: boolean
  // The automation keeps a learning journal (read it via DispatcherService.getAutomationJournal).
  journal?: boolean
  // Prompt profile id; empty runs the default for automations.
  profile?: string
}

// AutomationJournal is the learning journal an automation keeps between runs.
export interface AutomationJournal {
  journal: string
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
  /** The --record flag: writes top-level recordings and turns on the Recordings page. */
  enabled: boolean
  dir: string
  /** Run recordings (written while run logging is on) can be listed and read. */
  run_recordings?: boolean
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

// TriggerStatus is the dispatch admission outcome reported by the trigger
// endpoint: started immediately, queued behind running work, or skipped
// (a heartbeat with no checks has nothing to run).
export type TriggerStatus = 'started' | 'queued' | 'skipped'

export interface TriggerResponse {
  status: TriggerStatus
  position?: number
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
  // What "Allow and remember" would change, one line each; absent when there is nothing to
  // remember, and remember_reason says why. MUST stay in sync with GuardrailBlockedPayload in
  // backend/internal/core/assistant/agent_events.go.
  remember?: string[]
  remember_reason?: string
}

// UpstreamEventPayload describes a transient upstream LLM failure that is being
// retried. Emitted on the 'upstream' AgentEventType. MUST stay in sync with
// UpstreamEventPayload in backend/internal/core/assistant/agent_events.go.
export interface UpstreamEventPayload {
  event: string // "retry"
  reason: 'transport' | 'status' | 'model_starting' | 'model_busy'
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
  // The automation that produced the event (automation channel only); the stream is
  // routed per automation, so a console only receives its own automation's events.
  automation?: string
  payload: AgentStepStartPayload | AgentMessagePayload | AgentToolCallPayload | AgentToolResultPayload | AgentGuardrailViolationPayload | GuardrailBlockedPayload | UpstreamEventPayload | string
  timestamp?: string
}


// One saved final report of an automation run (reports/<automation>/<start time>-<run id>.md).
export interface SavedReport {
  path: string
  run_id: string
  started_at: string
  size_bytes: number
}

// An automation's saved reports, newest first; truncated when older ones were left out.
export interface SavedReports {
  reports: SavedReport[]
  truncated: boolean
}
