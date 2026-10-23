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
  | { kind: 'tool_call', name: string, args: string, status: 'running' | 'success' | 'error', result?: string, error?: string, helpers?: HelperProgress[] }
  | { kind: 'guardrail', tool: string, error: string }
  | { kind: 'error', message: string }
  | { kind: 'notice', message: string, status?: 'pending' | 'resolved' }

// A delegated sub-agent's live progress inside its delegate_task step: what it was asked, how far it got, how it
// ended. Built from 'subagent' events; not stored, so a reloaded chat shows the step's result instead.
export interface HelperProgress {
  runId: string
  agent: string
  task: string
  steps: number
  lastStep: string
  status: 'running' | 'done' | 'busy' | 'refused' | 'failed' | 'stopped'
}

// How one delegated task ran (backend models.SubagentRun); its tokens are not in the turn's own counts.
export interface SubagentRun {
  agent: string
  model?: string
  status: string
  duration_ms?: number
  prompt_tokens?: number
  completion_tokens?: number
}

// How a chat turn ran, stored by the backend on the turn's user message
// (models.TurnRun). Old sessions have none; every field is optional. Token
// counts are the provider's own, absent when it reported none.
export interface TurnRun {
  model?: string
  started_at?: string
  duration_ms?: number
  prompt_tokens?: number
  completion_tokens?: number
  // Facts saved from the user's message before the run (an explicit "remember …").
  memory_saved?: string[]
  // Non-fatal problems in the turn, e.g. a tool whose credential was rejected.
  warnings?: string[]
  // Requests the server rejected as too large; each was answered by cutting the history.
  context_overflows?: number
  // The tasks the turn delegated to sub-agents.
  subagents?: SubagentRun[]
}

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
  // On a user message: how the turn it started ran.
  run?: TurnRun
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
  // The prompt profile this conversation runs under; absent for one that never chose.
  profile?: string
}

// What the operator picked for the next message: a registry model and a prompt profile ('' = no choice).
export interface RunChoice {
  model?: string
  profile?: string
  /** An agent for this message only (a turn command such as /research); the conversation keeps its own profile. */
  agent?: string
  /** Ask the assistant to hand the independent parts of this message to helpers (sub-agents). */
  useHelpers?: boolean
}

export interface ChatRequestPayload {
  workspace_id: string
  conversation_id?: string
  message: string
  context_version?: string
  timezone?: string
  // A registry model chosen for this message; absent runs the primary.
  model?: string
  // A prompt profile chosen for this message, which the conversation then keeps; absent keeps the conversation's own.
  profile?: string
  // An agent for this message only (a turn command); the conversation keeps its own profile.
  agent?: string
  // The user switched helpers on for this message (the composer's "Use helpers"); absent means they did not.
  use_helpers?: boolean
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
  // The conversation a running chat serves (chat holders only; absent while unknown).
  conversation_id?: string
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

// GlobalRunTick records one poll of the global lane snapshot (plan D20): an
// 'error' tick carries the previous good holders and must never be read as
// "every run ended". seq increases on every tick, so each one is observable.
export type RunTickStatus = 'ok' | 'error'
export interface GlobalRunTick {
  seq: number
  status: RunTickStatus
  holders: LaneHolder[]
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
  lanes?: LaneSummary[]
}

// One run lane's capacity and occupancy (handlers.LaneSummary). holder_keys
// match lane_holders[].key; inbound callers hold no lane slot.
export interface LaneSummary {
  lane: LaneKey
  limit: number
  running: number
  waiting: number
  holder_keys: string[]
}

import type { LifecyclePhase } from './dispatcher'

// How a chat run ended. `completed` is only an answer that arrived: a run that raised an error is `failed` (the server
// still sends the completed lifecycle after it) and one the user stopped is `stopped`.
export type ChatRunOutcome = 'completed' | 'failed' | 'stopped'

export interface RunEnd {
  /** The run's token (useAssistant's runToken when it started). */
  token: number
  outcome: ChatRunOutcome
}

// The message the user wrote while a run was live: it waits behind that run (its token) in that conversation, and is sent
// only when the run ended as completed. One at a time.
export interface QueuedMessage {
  text: string
  workspaceId: string
  /** The conversation it was written in; null for a first message whose conversation has no id yet. */
  conversationId: string | null
  runToken: number
}

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

// A headed group in the conversation list (utils/assistant/sessionSections.ts):
// pinned conversations, manual ones by recency, or a source (webhook) folder.
export interface SessionSection {
  key: string
  label: string
  kind: 'pinned' | 'recency' | 'source'
  sessions: SessionBrief[]
}
