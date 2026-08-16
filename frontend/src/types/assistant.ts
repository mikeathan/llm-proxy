export type AssistantRole = 'system' | 'user' | 'assistant' | 'tool'

export interface ToolCall {
  id: string
  type: string
  function: {
    name: string
    arguments: string
  }
}

export type Segment =
  | { kind: 'reasoning', text: string }
  | { kind: 'tool_call', name: string, args: string, status: 'running' | 'success' | 'error', result?: string, error?: string }
  | { kind: 'guardrail', tool: string, error: string }
  | { kind: 'error', message: string }

export interface AssistantMessage {
  role: AssistantRole
  content: string
  reasoning_content?: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  toolResult?: { name: string; result: any; error?: string }
  segments?: Segment[]
  canceled?: boolean
}

export interface SessionBrief {
  id: string
  snippet: string
  updated_at: string
  running?: boolean
  // source is supplied by the backend ("webhook-<platform>" or "manual").
  source?: string
}

export interface AssistantSession {
  id: string
  workspace_id: string
  context_version: string
  timezone: string
  history: AssistantMessage[]
  metadata?: Record<string, any>
  cancelled_indices?: number[]
  updated_at: string
}

export interface ChatRequestPayload {
  workspace_id: string
  conversation_id?: string
  message: string
  context_version?: string
  timezone?: string
}

import type { AgentEvent } from './dispatcher'

// ChatResponsePayload is the response to POST /conversation/message. The
// backend starts a detached background run and returns immediately with
// status:"running"; the run is observed live over SSE. `reply` is only
// populated if the run somehow completed synchronously (legacy/edge cases) —
// normally it is absent and the SSE lifecycle{completed} event finalizes the
// turn.
export interface ChatResponsePayload {
  status?: string
  reply?: string
  conversation_id: string
  events?: AgentEvent[]
  canceled?: boolean
}

export interface CancelAgentResponse {
  conversation_id: string
  canceled: boolean
}

// ActiveRunsResponse is the authoritative per-workspace "currently executing"
// state returned by GET /workspaces/{id}/active-runs. It is the single source
// the UI polls to drive "running" notifications instead of sticky local flags.
export interface ActiveRunsResponse {
  assistant_running: boolean
  automation_running: boolean
}
