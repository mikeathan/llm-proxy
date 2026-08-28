import { describe, it, expect } from 'vitest'
import { LIFECYCLE_PHASES } from '../../types/dispatcher'
import { isFinishedPhase, isRunningPhase, isStreamingPhase } from '../../types/inset'

// These exact string values are sent verbatim by the Go backend over SSE
// (backend/internal/core/assistant/agent_events.go) — pinning them here guards
// against accidental renames that would silently break the wire.
describe('LIFECYCLE_PHASES wire values', () => {
  it('pins the backend contract values', () => {
    expect(LIFECYCLE_PHASES).toEqual({
      agentThinking: 'agent_thinking',
      stillThinking: 'still_thinking',
      sessionStarted: 'session_started',
      sessionProgress: 'session_progress',
      sessionCompleted: 'session_completed',
      completed: 'completed',
    })
  })
})

describe('InsetPhase predicates', () => {
  it('isStreamingPhase covers thinking/working only', () => {
    expect(isStreamingPhase('thinking')).toBe(true)
    expect(isStreamingPhase('working')).toBe(true)
    expect(isStreamingPhase('generating')).toBe(false)
    expect(isStreamingPhase('done')).toBe(false)
    expect(isStreamingPhase('idle')).toBe(false)
  })

  it('isRunningPhase adds generating', () => {
    expect(isRunningPhase('thinking')).toBe(true)
    expect(isRunningPhase('working')).toBe(true)
    expect(isRunningPhase('generating')).toBe(true)
    expect(isRunningPhase('done')).toBe(false)
    expect(isRunningPhase('idle')).toBe(false)
  })

  it('isFinishedPhase is done only', () => {
    expect(isFinishedPhase('done')).toBe(true)
    expect(isFinishedPhase('idle')).toBe(false)
    expect(isFinishedPhase('thinking')).toBe(false)
  })
})
