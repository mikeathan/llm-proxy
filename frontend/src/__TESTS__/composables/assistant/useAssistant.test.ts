import { describe, it, expect, vi, beforeEach } from 'vitest'
import { nextTick, ref } from 'vue'
import type { AgentEvent } from '../../../types'

// The network edges are faked; everything between them (SSE dedup, the
// message builder, session bookkeeping) is the real code under test.
const sse = {
  isConnected: ref(false),
  onMessage: null as ((ev: AgentEvent) => void) | null,
  connect: vi.fn(),
  disconnect: vi.fn(),
}
vi.mock('../../../composables/network/useSSEConnection', () => ({
  useSSEConnection: (opts: { onMessage: (ev: AgentEvent) => void }) => {
    sse.onMessage = opts.onMessage
    return { isConnected: sse.isConnected, connect: sse.connect, disconnect: sse.disconnect }
  },
}))
const service = {
  sendMessage: vi.fn(),
  listSessions: vi.fn(),
  getSession: vi.fn(),
  deleteSession: vi.fn(),
  cancelAgent: vi.fn(),
  submitGuardrailDecision: vi.fn(),
}
vi.mock('../../../services/assistant/assistantService', () => ({ AssistantService: service }))
const banner = { show: vi.fn(), clear: vi.fn() }
vi.mock('../../../composables/ui/useAppBanner', () => ({ useAppBanner: () => banner }))

const lifecycle = (phase: string, conversation_id: string, extra: Record<string, unknown> = {}): AgentEvent =>
  ({ type: 'lifecycle', channel: 'assistant', payload: { phase, conversation_id, workspace_id: 'ws', ...extra } }) as unknown as AgentEvent

// A fresh singleton per test: useAssistant keeps module-level state.
async function freshAssistant() {
  vi.resetModules()
  const { useAssistant } = await import('../../../composables/assistant/useAssistant')
  return useAssistant()
}

// Characterised before the Assistant chrome row (plan D22).
describe('useAssistant', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sse.isConnected.value = false
    service.sendMessage.mockResolvedValue({ conversation_id: 'c1', status: 'running' })
    service.listSessions.mockResolvedValue([])
    service.deleteSession.mockResolvedValue(undefined)
  })

  it('connects the live stream before starting the run', async () => {
    const a = await freshAssistant()
    const sending = a.sendMessage('ws', 'hello')
    await nextTick()
    expect(sse.connect).toHaveBeenCalled()
    expect(service.sendMessage).not.toHaveBeenCalled()
    sse.isConnected.value = true
    await sending
    expect(service.sendMessage).toHaveBeenCalledWith({ workspace_id: 'ws', conversation_id: undefined, message: 'hello' })
  })

  it('sends the model picked for the message, and none when the default is chosen', async () => {
    sse.isConnected.value = true
    const a = await freshAssistant()
    await a.sendMessage('ws', 'hello', { model: 'nemotron' })
    expect(service.sendMessage).toHaveBeenLastCalledWith(expect.objectContaining({ model: 'nemotron' }))
    await a.sendMessage('ws', 'again')
    expect(service.sendMessage.mock.lastCall![0].model).toBeUndefined()
    expect(service.sendMessage.mock.lastCall![0].profile).toBeUndefined()
  })

  it('sends the profile picked for the message', async () => {
    sse.isConnected.value = true
    const a = await freshAssistant()
    await a.sendMessage('ws', 'hello', { profile: 'research' })
    expect(service.sendMessage).toHaveBeenLastCalledWith(expect.objectContaining({ profile: 'research' }))
  })

  it('sends use_helpers only when the message asked for helpers', async () => {
    sse.isConnected.value = true
    const a = await freshAssistant()
    await a.sendMessage('ws', 'hello', { useHelpers: true })
    expect(service.sendMessage).toHaveBeenLastCalledWith(expect.objectContaining({ use_helpers: true }))
    await a.sendMessage('ws', 'again', { useHelpers: false })
    expect(service.sendMessage.mock.lastCall![0].use_helpers).toBeUndefined()
  })

  it('keeps the helpers switch for the open conversation and clears it for another or a new one', async () => {
    const a = await freshAssistant()
    a.helpersOn.value = true
    service.getSession.mockResolvedValue({ id: 's1', history: [{ role: 'user', content: 'q' }] })
    await a.loadSession('ws', 's1')
    expect(a.helpersOn.value, 'a conversation opened from the list starts with helpers off').toBe(false)

    a.helpersOn.value = true
    a.newSession()
    expect(a.helpersOn.value, 'so does a new chat').toBe(false)
  })

  it("takes the open conversation's profile from the saved session, and forgets it for a new chat", async () => {
    const a = await freshAssistant()
    service.getSession.mockResolvedValue({ id: 's1', profile: 'builder', history: [{ role: 'user', content: 'q' }] })
    await a.loadSession('ws', 's1')
    expect(a.conversationProfile.value).toBe('builder')

    service.getSession.mockResolvedValue({ id: 's2', history: [{ role: 'user', content: 'q' }] })
    await a.loadSession('ws', 's2')
    expect(a.conversationProfile.value, 'a conversation that never chose has none').toBe('')

    a.conversationProfile.value = 'research'
    a.newSession()
    expect(a.conversationProfile.value).toBe('')
  })

  it('does not claim a profile for a conversation opened while it runs', async () => {
    const a = await freshAssistant()
    a.conversationProfile.value = 'research'
    a.sessions.value = [{ id: 's1', snippet: 'x', updated_at: '', running: true }]
    await a.loadSession('ws', 's1')
    expect(a.conversationProfile.value, 'unknown, so nothing is sent and the conversation keeps its own').toBe('')
  })

  it('adopts the new conversation and lists it first, staying busy until the run completes', async () => {
    sse.isConnected.value = true
    const a = await freshAssistant()
    await a.sendMessage('ws', 'hello')
    expect(a.currentSessionId.value).toBe('c1')
    expect(a.sessions.value[0]).toMatchObject({ id: 'c1', snippet: 'hello' })
    expect(a.loading.value).toBe(true)

    sse.onMessage!(lifecycle('session_completed', 'c1'))
    expect(a.loading.value).toBe(false)
  })

  it('ignores content from another conversation on the shared socket', async () => {
    sse.isConnected.value = true
    const a = await freshAssistant()
    await a.sendMessage('ws', 'hello')
    const before = JSON.stringify(a.messages.value)
    sse.onMessage!({ type: 'message', channel: 'assistant', conversation_id: 'other', payload: { content: 'leak' } } as unknown as AgentEvent)
    expect(JSON.stringify(a.messages.value)).toBe(before)
  })

  it('reports a failed send and stops being busy', async () => {
    sse.isConnected.value = true
    service.sendMessage.mockRejectedValue(new Error('no model configured'))
    const a = await freshAssistant()
    await a.sendMessage('ws', 'hello')
    expect(banner.show).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error', message: 'no model configured' }))
    expect(a.loading.value).toBe(false)
  })

  it('shows a run started elsewhere (a webhook) as a new turn', async () => {
    const a = await freshAssistant()
    a.activeWorkspaceId.value = 'ws'
    sse.onMessage!(lifecycle('session_started', 'hook-1', { snippet: 'from telegram', source: 'webhook' }))
    expect(a.currentSessionId.value).toBe('hook-1')
    expect(a.messages.value).toEqual([{ role: 'user', content: 'from telegram' }])
    expect(a.sessions.value[0]).toMatchObject({ id: 'hook-1', running: true })
  })

  it('loads a saved conversation, marking cancelled turns', async () => {
    service.getSession.mockResolvedValue({
      id: 's1',
      history: [{ role: 'user', content: 'q' }, { role: 'assistant', content: 'a' }],
      cancelled_indices: [0],
    })
    const a = await freshAssistant()
    await a.loadSession('ws', 's1')
    expect(a.currentSessionId.value).toBe('s1')
    expect(a.messages.value[0]).toMatchObject({ role: 'user', content: 'q', canceled: true })
    expect(a.phase.value).toBe('done')
  })

  it('deleting the open conversation starts a fresh one', async () => {
    service.getSession.mockResolvedValue({ id: 's1', history: [{ role: 'user', content: 'q' }] })
    const a = await freshAssistant()
    a.sessions.value = [{ id: 's1', snippet: 'q', updated_at: '' }, { id: 's2', snippet: 'r', updated_at: '' }]
    await a.loadSession('ws', 's1')
    await a.deleteSession('ws', 's1')
    expect(a.sessions.value.map((s) => s.id)).toEqual(['s2'])
    expect(a.currentSessionId.value).toBeNull()
    expect(a.messages.value).toEqual([])
  })
})

describe('useAssistantSSE', () => {
  beforeEach(() => vi.clearAllMocks())

  async function freshSSE() {
    vi.resetModules()
    const { useAssistantSSE } = await import('../../../composables/assistant/useAssistantSSE')
    const onEvent = vi.fn()
    const onSession = vi.fn()
    return { s: useAssistantSSE(() => 'ws', onEvent, onSession), onEvent, onSession }
  }

  it('passes assistant events on once, dropping other channels and repeats', async () => {
    const { onEvent } = await freshSSE()
    sse.onMessage!({ id: 'e1', type: 'message', channel: 'assistant', payload: {} } as unknown as AgentEvent)
    sse.onMessage!({ id: 'e1', type: 'message', channel: 'assistant', payload: {} } as unknown as AgentEvent)
    sse.onMessage!({ id: 'e2', type: 'message', channel: 'automation', payload: {} } as unknown as AgentEvent)
    expect(onEvent).toHaveBeenCalledTimes(1)
  })

  it('forwards lifecycle phases as session updates', async () => {
    const { onSession } = await freshSSE()
    sse.onMessage!(lifecycle('session_started', 'c1', { snippet: 'hi' }))
    expect(onSession).toHaveBeenCalledWith(expect.objectContaining({ phase: 'session_started', conversation_id: 'c1', snippet: 'hi' }))
  })

  it('raises a guardrail approval once, and drops it when the backend withdraws it', async () => {
    const { s } = await freshSSE()
    sse.onMessage!({ id: 'g1', type: 'guardrail_blocked', channel: 'assistant', payload: { decision_id: 'd1' } } as unknown as AgentEvent)
    expect(s.pendingDecision.value).toMatchObject({ decision_id: 'd1' })
    sse.onMessage!({ id: 'g2', type: 'guardrail_invalidated', channel: 'assistant', payload: { decision_id: 'd1' } } as unknown as AgentEvent)
    expect(s.pendingDecision.value).toBeNull()
    // A replay of the withdrawn approval is not shown again.
    sse.onMessage!({ id: 'g3', type: 'guardrail_blocked', channel: 'assistant', payload: { decision_id: 'd1' } } as unknown as AgentEvent)
    expect(s.pendingDecision.value).toBeNull()
  })

  it('keeps at most 1000 live events', async () => {
    const { s } = await freshSSE()
    for (let i = 0; i < 1005; i++) sse.onMessage!({ id: `e${i}`, type: 'message', channel: 'assistant', payload: {} } as unknown as AgentEvent)
    expect(s.liveEvents.value).toHaveLength(1000)
    expect(s.liveEvents.value[0]!.id).toBe('e5')
  })

  // A queued message waits for a particular run and is sent only when that run ended as completed, so the composable says
  // how each run ended. `loading` is not that signal: fetchSessions and a delete toggle it while a run is still live.
  describe('how a run ended', () => {
    const errorEvent = () => ({ type: 'error', channel: 'assistant', payload: { message: 'boom' } }) as unknown as AgentEvent

    async function startedRun() {
      sse.isConnected.value = true
      const a = await freshAssistant()
      await a.sendMessage('ws', 'hello')
      sse.onMessage!(lifecycle('session_started', 'c1'))
      return a
    }

    it('gives each run its own token and reports no ending while it is live', async () => {
      const a = await startedRun()
      const first = a.runToken.value
      expect(a.runEnd.value).toBeNull()
      sse.onMessage!(lifecycle('session_completed', 'c1'))
      await a.sendMessage('ws', 'again')
      expect(a.runToken.value).toBeGreaterThan(first)
    })

    it('reports a run that finished as completed', async () => {
      const a = await startedRun()
      const token = a.runToken.value
      sse.onMessage!(lifecycle('session_completed', 'c1'))
      expect(a.runEnd.value).toEqual({ token, outcome: 'completed' })
    })

    it('reports a run that raised an error as failed, even though a completed event follows it', async () => {
      const a = await startedRun()
      const token = a.runToken.value
      sse.onMessage!(errorEvent())
      sse.onMessage!(lifecycle('session_completed', 'c1'))
      expect(a.runEnd.value).toEqual({ token, outcome: 'failed' })
    })

    it('reports a run the user stopped as stopped', async () => {
      const a = await startedRun()
      const token = a.runToken.value
      await a.cancel()
      expect(a.runEnd.value).toEqual({ token, outcome: 'stopped' })
    })

    it('reports stopped even when the completed event arrives before the stop request returns', async () => {
      const a = await startedRun()
      const token = a.runToken.value
      let answerStop: (value: { conversation_id: string }) => void = () => {}
      service.cancelAgent.mockReturnValueOnce(new Promise((resolve) => { answerStop = resolve }))
      const stopping = a.cancel()
      sse.onMessage!(lifecycle('session_completed', 'c1')) // the server says it is over before it answers the request
      answerStop({ conversation_id: 'c1' })
      await stopping
      expect(a.runEnd.value).toEqual({ token, outcome: 'stopped' })
    })

    it('does not read a refresh of the session list as the run ending, although it toggles loading', async () => {
      const a = await startedRun()
      await a.fetchSessions('ws')
      expect(a.loading.value).toBe(false)
      expect(a.runEnd.value).toBeNull()
    })

    it('ignores another conversation finishing', async () => {
      const a = await startedRun()
      sse.onMessage!(lifecycle('session_completed', 'someone-else'))
      expect(a.runEnd.value).toBeNull()
    })
  })
})
