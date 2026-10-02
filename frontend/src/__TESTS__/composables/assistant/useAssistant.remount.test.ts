import { describe, it, expect, vi, beforeEach } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import type { AgentEvent } from '../../../types'

// A fake of the live stream that behaves like the real one where it matters for
// this bug: every useSSEConnection() call is its own connection; connect() closes
// that connection's previous socket and opens a new one, which REPLAYS the run's
// recent events (session_started first) before delivering live events; a closed
// connection receives nothing. Everything between this edge and the UI is real.
interface FakeConnection {
  onMessage: (ev: AgentEvent) => void
  open: boolean
  isConnected: ReturnType<typeof ref<boolean>>
}
const connections: FakeConnection[] = []
let recent: AgentEvent[] = []

vi.mock('../../../composables/network/useSSEConnection', () => ({
  useSSEConnection: (opts: { onMessage: (ev: AgentEvent) => void }) => {
    const conn: FakeConnection = { onMessage: opts.onMessage, open: false, isConnected: ref(false) }
    connections.push(conn)
    return {
      isConnected: conn.isConnected,
      connect: () => {
        conn.open = true
        conn.isConnected.value = true
        // The server replays the run so far to a NEW subscriber, then streams live.
        for (const ev of recent) conn.onMessage(ev)
      },
      disconnect: () => {
        conn.open = false
        conn.isConnected.value = false
      },
    }
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

let nextId = 0
const ev = (type: string, payload: unknown, conversation_id = 'run-1'): AgentEvent =>
  ({ id: `e${++nextId}`, type, channel: 'assistant', conversation_id, payload }) as unknown as AgentEvent
const lifecycle = (phase: string, extra: Record<string, unknown> = {}): AgentEvent =>
  ev('lifecycle', { phase, conversation_id: 'run-1', workspace_id: 'ws', ...extra })

// What the server holds for a run in progress: session_started, then content.
const runSoFar = () => [
  lifecycle('session_started', { snippet: 'do the long task' }),
  ev('reasoning', 'planning the work'),
  ev('tool_call', { id: 'c1', type: 'function', function: { name: 'read_file', arguments: '{"path":"a.txt"}' } }),
  ev('tool_result', { id: 'c1', name: 'read_file', result: 'file a' }),
]

// A further step of the same run, arriving live after the replay.
const liveStep = (): AgentEvent[] => [
  ev('tool_call', { id: 'c2', type: 'function', function: { name: 'read_file', arguments: '{"path":"b.txt"}' } }),
  ev('tool_result', { id: 'c2', name: 'read_file', result: 'file b' }),
]

// A chat "page visit": a fresh effect scope standing in for the mounted
// AssistantChat component, so leaving the page can stop it like Vue does.
async function visit() {
  const { useAssistant } = await import('../../../composables/assistant/useAssistant')
  const scope = effectScope()
  const assistant = scope.run(() => useAssistant())!
  return { scope, assistant }
}

// What AssistantChat's onMounted does (initWorkspace), then the history click.
async function mountChat(assistant: Awaited<ReturnType<typeof visit>>['assistant']) {
  assistant.activeWorkspaceId.value = 'ws'
  assistant.newSession()
  await assistant.fetchSessions('ws')
  assistant.connectSSE()
}

const userAnchors = (a: Awaited<ReturnType<typeof visit>>['assistant']) =>
  a.messages.value.filter((m) => m.role === 'user' && m.content === 'do the long task').length

describe('returning to a chat whose run is still going', () => {
  beforeEach(async () => {
    vi.resetModules()
    vi.clearAllMocks()
    connections.length = 0
    nextId = 0
    recent = runSoFar()
    service.listSessions.mockResolvedValue([{ id: 'run-1', snippet: 'do the long task', updated_at: '2026-10-01T10:00:00Z' }])
    service.getSession.mockResolvedValue(null) // not on disk yet: a first turn that is still running
  })

  it('stops the first visit listening when the chat unmounts, so only one live connection remains', async () => {
    const first = await visit()
    await mountChat(first.assistant)
    first.assistant.reconcileRunningConversation('run-1')
    expect(connections.filter((c) => c.open)).toHaveLength(1)

    first.scope.stop() // the user moves to another page
    expect(connections.filter((c) => c.open), 'the unmounted chat must not keep a live connection').toHaveLength(0)

    const second = await visit()
    await mountChat(second.assistant)
    expect(connections.filter((c) => c.open)).toHaveLength(1)
  })

  const toolCalls = (a: Awaited<ReturnType<typeof visit>>['assistant'], path: string) =>
    a.messages.value.flatMap((m) => m.segments ?? []).filter((seg) => seg.kind === 'tool_call' && String(seg.args).includes(path)).length

  it('shows each step of the running turn exactly once after leaving and opening it from the history list', async () => {
    const first = await visit()
    await mountChat(first.assistant)
    first.assistant.reconcileRunningConversation('run-1')
    first.scope.stop() // away on another page while the run goes on

    const second = await visit()
    await mountChat(second.assistant)
    second.assistant.reconcileRunningConversation('run-1')
    await second.assistant.loadSession('ws', 'run-1') // the click on the running conversation

    expect(second.assistant.messages.value.length, 'the running turn must appear').toBeGreaterThan(0)
    expect(userAnchors(second.assistant), 'the task appears once').toBe(1)
    expect(toolCalls(second.assistant, 'a.txt'), 'replayed step shown once').toBe(1)

    // The run keeps going: a live step must extend the turn once — not twice, not wiped.
    const live = liveStep()
    recent.push(...live)
    for (const e of live) for (const c of connections.filter((c) => c.open)) c.onMessage(e)
    await nextTick()
    expect(toolCalls(second.assistant, 'b.txt'), 'a live step must appear exactly once').toBe(1)
    expect(toolCalls(second.assistant, 'a.txt')).toBe(1)
    expect(userAnchors(second.assistant)).toBe(1)
  })

  it('looks the same as a chat that was never left', async () => {
    const control = await visit()
    await mountChat(control.assistant)
    control.assistant.reconcileRunningConversation('run-1')
    for (const e of liveStep()) for (const c of connections.filter((c) => c.open)) c.onMessage(e)
    const expected = JSON.stringify(control.assistant.messages.value)

    vi.resetModules()
    connections.length = 0
    nextId = 0
    recent = runSoFar()
    const first = await visit()
    await mountChat(first.assistant)
    first.assistant.reconcileRunningConversation('run-1')
    first.scope.stop()
    const second = await visit()
    await mountChat(second.assistant)
    second.assistant.reconcileRunningConversation('run-1')
    await second.assistant.loadSession('ws', 'run-1')
    const live = liveStep()
    for (const e of live) for (const c of connections.filter((c) => c.open)) c.onMessage(e)
    await nextTick()
    expect(JSON.stringify(second.assistant.messages.value)).toBe(expected)
  })
})
