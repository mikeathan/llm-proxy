import { describe, it, expect } from 'vitest'
import { automationTarget, historicalRunTarget, runTarget, runTitle } from '../../../utils/runs/runTarget'
import type { LaneHolder, QueuedRun } from '../../../types/assistant'
import type { AutomationRun } from '../../../types/dispatcher'

const holder = (over: Partial<LaneHolder>): LaneHolder => ({ key: 'k', kind: 'automation', workspace_id: 'ws', label: 'ws/nightly', since: '2026-10-01T10:00:00Z', ...over })
const queued = (over: Partial<QueuedRun>): QueuedRun => ({ key: 'k', workspace_id: 'ws', label: 'ws/nightly', position: 1, queued_at: '', ...over })

// Where a running (or waiting) row should take the operator. The Overview and the
// header pill both list running work; each row is a link when there is somewhere to go.
describe('runTarget', () => {
  it('opens the exact conversation of a running chat', () => {
    expect(runTarget(holder({ kind: 'interactive', workspace_id: 'ws-1', conversation_id: 'conv-42', label: 'chat:ws-1' })))
      .toEqual({ name: 'workspace-assistant', params: { ws: 'ws-1', conversationId: 'conv-42' } })
  })

  it('opens the workspace chat when the conversation is not known yet', () => {
    expect(runTarget(holder({ kind: 'interactive', workspace_id: 'ws-1' })))
      .toEqual({ name: 'workspace-assistant', params: { ws: 'ws-1' } })
  })

  it('opens the automation, whose page shows the live run', () => {
    expect(runTarget(holder({ kind: 'automation', workspace_id: 'ws-1', automation: 'nightly' })))
      .toEqual({ name: 'automation', params: { id: 'ws-1/nightly' } })
  })

  it('links a queued automation and a queued chat the same way', () => {
    expect(runTarget(queued({ kind: 'automation', workspace_id: 'ws-1', automation: 'nightly' })))
      .toEqual({ name: 'automation', params: { id: 'ws-1/nightly' } })
    expect(runTarget(queued({ kind: 'interactive', workspace_id: 'ws-1' })))
      .toEqual({ name: 'workspace-assistant', params: { ws: 'ws-1' } })
  })

  it('has nowhere to go for an external API caller, or for a row it cannot place', () => {
    expect(runTarget(holder({ kind: 'inbound', workspace_id: '' }))).toBeNull()
    expect(runTarget(holder({ kind: 'automation', automation: undefined }))).toBeNull()
    expect(runTarget(holder({ kind: 'interactive', workspace_id: '' }))).toBeNull()
  })
})

describe('runTitle', () => {
  it('names a chat by its workspace instead of the scheduler key', () => {
    expect(runTitle(holder({ kind: 'interactive', workspace_id: 'ws-1', label: 'chat:ws-1' }))).toBe('Chat in ws-1')
  })

  it('keeps the label of an automation or an API caller', () => {
    expect(runTitle(holder({ kind: 'automation', label: 'ws-1/nightly' }))).toBe('ws-1/nightly')
    expect(runTitle(holder({ kind: 'inbound', label: 'curl' }))).toBe('curl')
  })
})

const finished = (over: Partial<AutomationRun> = {}): AutomationRun =>
  ({ id: 'run_1', workspace_id: 'ws-1', automation_name: 'nightly', timestamp: '', error: '', output: '', duration_ms: 1, model: '', ...over })

// A finished run lives in the Activity ledger; its automation has its own page.
describe('historicalRunTarget', () => {
  it('opens the run in Activity', () => {
    expect(historicalRunTarget(finished())).toEqual({ name: 'activity', query: { run: 'run_1' } })
  })
})

describe('automationTarget', () => {
  it('opens the automation a finished run belongs to', () => {
    expect(automationTarget(finished())).toEqual({ name: 'automation', params: { id: 'ws-1/nightly' } })
  })

  it('has nowhere to go for a run without its workspace', () => {
    expect(automationTarget(finished({ workspace_id: '' }))).toBeNull()
  })
})
