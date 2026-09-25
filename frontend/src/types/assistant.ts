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
  | { kind: 'notice', message: string, status?: 'pending' | 'resolved' }

export interface AssistantMessage {
  role: AssistantRole
  content: string
  reasoning_content?: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  toolResult?: { name: string; result: any; error?: string }
  segments?: Segment[]
  canceled?: boolean
  // error carries a persisted terminal run failure (assistant-role message
  // written by the backend) and renders as a kind:'error' segment on reload.
  error?: string
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
// Run-scheduler lane state is not here — the lane snapshot is global, so it
// lives only on GlobalActiveRunsResponse.
//
// LaneHolder identifies a run currently occupying a run-scheduler slot.
// LaneKind / LaneKey mirror the backend runlane enums (runlane.Kind / LaneKey).
// 'inbound' is an external /v1 caller holding the local model.
export type LaneKind = 'automation' | 'interactive' | 'inbound'
export type LaneKey = 'local' | 'cloud'

export interface LaneHolder {
  key: string
  kind: LaneKind
  workspace_id: string
  automation?: string
  label: string
  // model is the local model the holder is using (absent for cloud runs); the
  // residency gate refuses evicting it out from under the holder.
  model?: string
  since: string
}

// QueuedRun identifies a run waiting in a scheduler lane, or an external caller
// waiting for the local model (kind 'inbound' — the operator can promote or
// cancel it). position is 1-based within its own queue.
export interface QueuedRun {
  key: string
  kind?: LaneKind
  lane?: LaneKey
  workspace_id: string
  automation?: string
  label: string
  model?: string
  position: number
  queued_at: string
}

export interface ActiveRunsResponse {
  assistant_running: boolean
  automation_running: boolean
  // assistant_conversation_id is the conversation ID of the agent currently
  // running, or "" when none is running. The frontend uses it to mark the
  // correct history row as running after a refresh.
  assistant_conversation_id: string
  // assistant_queued is true while a chat run is registered but not yet
  // admitted to the run scheduler (assistant_running stays true while waiting).
  assistant_queued?: boolean
}

// GlobalActiveRunsResponse is the workspace-independent slice of the active
// state returned by GET /admin/api/active-runs. Every run occupies a lane, so
// this snapshot is a complete "something is running, anywhere" signal and backs
// the always-visible header indicator.
export interface GlobalActiveRunsResponse {
  lane_holders?: LaneHolder[]
  queued?: QueuedRun[]
}

import type { LifecyclePhase } from './dispatcher'

// SessionLifecyclePayload is the parsed payload of a session lifecycle SSE
// event (session_started / session_progress / session_completed), used to
// drive UI session state without a full re-fetch.
export interface SessionLifecyclePayload {
  phase: LifecyclePhase
  conversation_id?: string
  workspace_id?: string
  snippet?: string
  source?: string
}

// SourceSection groups sessions by source (webhook vs manual) for the session
// list. See utils/assistant/source.ts.
export interface SourceSection {
  source: string
  sessions: SessionBrief[]
  grouped: boolean
}
