// InsetPhase drives the ChatBubble inset visibility during an agent turn. It is
// a purely client-side UI state derived from backend lifecycle events — NOT a
// wire value. The backend sends lifecycle phases (agent_thinking, still_thinking,
// session_completed, completed — see types/dispatcher.ts LifecyclePhase and
// backend/internal/core/assistant/agent_events.go); the SSE handler maps those
// onto this UI state. Keep the mapping in utils/message/messageBuilder.ts in
// sync with the lifecycle events.
//   idle       — no activity yet
//   thinking   — reasoning/tool_stream arriving, no tool call dispatched
//   working    — at least one tool_call dispatched
//   generating — assistant message with content + no tool calls arrived
//   done       — turn finalized (finalize called)
export type InsetPhase = 'idle' | 'thinking' | 'working' | 'generating' | 'done'

// Named predicates for InsetPhase — prefer these over repeated inline
// `phase === 'thinking' || phase === 'working'` comparisons scattered across
// components, so the phase-set membership rules live in one place.
export const isStreamingPhase = (p: InsetPhase): boolean =>
  p === 'thinking' || p === 'working'

export const isRunningPhase = (p: InsetPhase): boolean =>
  isStreamingPhase(p) || p === 'generating'

export const isFinishedPhase = (p: InsetPhase): boolean => p === 'done'
