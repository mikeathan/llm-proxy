import { describe, expect, it, vi, beforeEach } from 'vitest'
import { effectScope } from 'vue'
import type { AgentEvent } from '../../../types'

const sse = { options: null as null | { url: () => string; onMessage: (ev: AgentEvent) => void }, connect: vi.fn(), disconnect: vi.fn(), reset: vi.fn() }
vi.mock('../../../composables/network/useSSEConnection', () => ({
  useSSEConnection: (options: { url: () => string; onMessage: (ev: AgentEvent) => void }) => {
    sse.options = options
    return { isConnected: { value: false }, connect: sse.connect, disconnect: sse.disconnect, reset: sse.reset }
  },
}))
vi.mock('../../../services/httpClient', () => ({ post: vi.fn().mockResolvedValue({}) }))

import { useLiveConsole } from '../../../composables/automation/useLiveConsole'

function setup(name = 'daily-llm-news') {
  const scope = effectScope()
  const console_ = scope.run(() => useLiveConsole(() => 'workspace-1', () => false, () => undefined, name))!
  return { console_, scope }
}

const blocked = (id: string): AgentEvent =>
  ({ id: `b-${id}`, type: 'guardrail_blocked', channel: 'automation', payload: { decision_id: id, tool: 'run_terminal', args: '', reason: 'x', category: 'terminal' } }) as unknown as AgentEvent
const invalidated = (id: string): AgentEvent =>
  ({ id: `i-${id}`, type: 'guardrail_invalidated', channel: 'automation', payload: { decision_id: id, reason: 'context_cancelled' } }) as unknown as AgentEvent

// A run's console shows that automation's run only: the workspace also runs the
// heartbeat and others, and their output once appeared on the news page.
describe('useLiveConsole', () => {
  beforeEach(() => vi.clearAllMocks())

  it('asks for its own automation\'s stream, encoded', () => {
    setup('daily llm/news')
    expect(sse.options!.url()).toBe('/admin/api/dispatcher/workspaces/workspace-1/live?channel=automation&automation=daily%20llm%2Fnews')
  })

  it('clears an approval banner when the matching decision is invalidated', () => {
    const { console_ } = setup()
    sse.options!.onMessage(blocked('d1'))
    expect(console_.pendingDecision.value).toMatchObject({ decision_id: 'd1' })
    sse.options!.onMessage(invalidated('d1'))
    expect(console_.pendingDecision.value).toBeNull()
  })

  it('labels the run with its own automation\'s name', () => {
    const { console_ } = setup('daily-llm-news')
    sse.options!.onMessage({ id: 'm1', type: 'message', channel: 'automation', payload: { role: 'system', content: '▶ Booting automation: daily-llm-news' } } as unknown as AgentEvent)
    expect(console_.messages.value[0]).toMatchObject({ role: 'user', content: 'Automation run: daily-llm-news' })
  })
})
