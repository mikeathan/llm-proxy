import { describe, it, expect } from 'vitest'
import { markRunningConversation, clearRunningFlags, reconcileRunningSessions } from '../../../utils/assistant/running'
import type { SessionBrief } from '../../../types/assistant'

function brief(id: string, running = false): SessionBrief {
  return { id, snippet: '', updated_at: '', running }
}

describe('markRunningConversation', () => {
  it('flags the matching session and unflags every other session', () => {
    const sessions = [brief('a'), brief('b', true), brief('c')]
    const next = markRunningConversation(sessions, 'c')

    expect(next.map((s) => s.running)).toEqual([false, false, true])
  })

  it('returns the same array identity when already correct (idempotent)', () => {
    const sessions = [brief('a', true), brief('b')]
    const next = markRunningConversation(sessions, 'a')
    expect(next).toBe(sessions)
  })

  it('is a no-op for an empty conversation id', () => {
    const sessions = [brief('a', true)]
    expect(markRunningConversation(sessions, '')).toBe(sessions)
  })

  it('clears a running flag when no session matches the id', () => {
    const sessions = [brief('a', true), brief('b')]
    const next = markRunningConversation(sessions, 'missing')
    expect(next.map((s) => s.running)).toEqual([false, false])
  })
})

describe('clearRunningFlags', () => {
  it('clears every running flag', () => {
    const sessions = [brief('a', true), brief('b', true), brief('c')]
    expect(clearRunningFlags(sessions).map((s) => s.running)).toEqual([false, false, false])
  })
})

describe('reconcileRunningSessions', () => {
  it('marks exactly the matching session when a conversation is running', () => {
    const sessions = [brief('a', true), brief('b'), brief('c', true)]
    const next = reconcileRunningSessions(sessions, 'b')
    expect(next.map((s) => s.running)).toEqual([false, true, false])
  })

  it('clears every running flag when no conversation is running (empty id)', () => {
    const sessions = [brief('a', true), brief('b')]
    const next = reconcileRunningSessions(sessions, '')
    expect(next.map((s) => s.running)).toEqual([false, false])
  })

  it('is idempotent when the state already matches', () => {
    const sessions = [brief('a', true), brief('b')]
    expect(reconcileRunningSessions(sessions, 'a')).toBe(sessions)

    const idle = [brief('c'), brief('d')]
    expect(reconcileRunningSessions(idle, '').map((s) => s.running)).toEqual([false, false])
  })
})
