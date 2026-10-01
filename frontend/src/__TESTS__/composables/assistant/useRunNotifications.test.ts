import { describe, it, expect, vi } from 'vitest'
import { nextTick, shallowRef } from 'vue'
import { createRunNotifications } from '../../../composables/assistant/useRunNotifications'
import { toAutomation, toWorkspace, toWorkspaceAssistant } from '../../../router/routes'
import type { GlobalRunTick, LaneHolder } from '../../../types/assistant'
import type { AutomationRun } from '../../../types/dispatcher'
import type { RunEndItem } from '../../../types/notifications'

const chat = (ws: string, since: string): LaneHolder => ({ key: `chat:${ws}`, kind: 'interactive', workspace_id: ws, label: ws, since })
const auto = (ws: string, name: string, since: string): LaneHolder => ({ key: `${ws}/${name}`, kind: 'automation', workspace_id: ws, automation: name, label: `${ws}/${name}`, since })
const record = (ws: string, name: string, timestamp: string, error = ''): AutomationRun =>
  ({ id: `r-${name}-${timestamp}`, workspace_id: ws, automation_name: name, timestamp, error, output: '', duration_ms: 1, model: 'm' })

function setup(opts: { ledger?: AutomationRun[][]; showing?: (item: RunEndItem) => boolean; conversation?: Record<string, string>; cap?: number } = {}) {
  const tick = shallowRef<GlobalRunTick | null>(null)
  const ledgers = [...(opts.ledger ?? [])]
  const fetchLedger = vi.fn(async () => ledgers.shift() ?? [])
  const announce = vi.fn()
  const n = createRunNotifications({
    lastTick: tick,
    fetchLedger,
    isShowing: opts.showing ?? (() => false),
    conversationFor: (ws) => opts.conversation?.[ws] ?? null,
    announce,
    cap: opts.cap,
  })
  let seq = 0
  const push = async (holders: LaneHolder[], status: GlobalRunTick['status'] = 'ok') => {
    tick.value = { seq: ++seq, status, holders }
    await nextTick()
    await n.settled()
  }
  return { n, push, fetchLedger, announce }
}

const items = (n: ReturnType<typeof createRunNotifications>) => n.notifications.value.flatMap((b) => b.items)

describe('run notifications', () => {
  it('takes the first good tick as the baseline: runs finished before load never notify', async () => {
    const { n, push } = setup()
    await push([], 'error')
    await push([chat('ws', 't1')])
    expect(items(n)).toEqual([])
    await push([])
    expect(items(n)).toHaveLength(1)
  })

  it('never reads a failed poll as runs ending', async () => {
    const { n, push } = setup()
    await push([chat('ws', 't1')])
    await push([], 'error')
    expect(items(n)).toEqual([])
    await push([chat('ws', 't1')])
    expect(items(n)).toEqual([])
  })

  it('labels an assistant run "ended" and targets its workspace when the conversation is unknown', async () => {
    const { n, push, fetchLedger } = setup()
    await push([chat('ws', 't1')])
    await push([])
    expect(items(n)[0]).toMatchObject({ kind: 'assistant', outcome: 'ended', workspace: 'ws', target: toWorkspace('ws'), destination: 'workspaces' })
    expect(fetchLedger).not.toHaveBeenCalled()
  })

  it('targets the conversation when it is known for that workspace', async () => {
    const { n, push } = setup({ conversation: { ws: 'c1' } })
    await push([chat('ws', 't1')])
    await push([])
    expect(items(n)[0]!.target).toEqual(toWorkspaceAssistant('ws', 'c1'))
  })

  it('notifies a second run in the same workspace', async () => {
    const { n, push } = setup()
    await push([chat('ws', 't1')])
    await push([chat('ws', 't2')])
    await push([])
    expect(items(n).map((i) => i.id)).toEqual(['chat:ws@t1', 'chat:ws@t2'])
  })

  it('does not notify what the user is already looking at', async () => {
    const { n, push } = setup({ showing: (item) => item.workspace === 'ws' })
    await push([chat('ws', 't1'), chat('other', 't1')])
    await push([])
    expect(items(n).map((i) => i.workspace)).toEqual(['other'])
  })

  it('ignores external callers holding the local model', async () => {
    const { n, push } = setup()
    await push([{ key: 'inbound:1', kind: 'inbound', workspace_id: '', label: 'm', since: 't1' }])
    await push([])
    expect(items(n)).toEqual([])
  })

  it('reads the run record once per batch, only on a transition, and reports a failure', async () => {
    const { n, push, fetchLedger } = setup({ ledger: [[record('ws', 'a', '2026-09-28T10:02:00Z', 'boom'), record('ws', 'a', '2026-09-28T09:00:00Z')]] })
    const running = [auto('ws', 'a', '2026-09-28T10:00:00Z'), chat('ws', '2026-09-28T10:00:00Z')]
    await push(running)
    await push(running)
    expect(fetchLedger).not.toHaveBeenCalled()
    await push([])
    expect(fetchLedger).toHaveBeenCalledTimes(1)
    expect(items(n).find((i) => i.kind === 'automation')).toMatchObject({ outcome: 'failed', error: 'boom', target: toAutomation('ws/a'), destination: 'automations' })
  })

  it('retries a missing run record once on the next tick, then says "ended"', async () => {
    const late = setup({ ledger: [[], [record('ws', 'a', '2026-09-28T10:02:00Z')]] })
    await late.push([auto('ws', 'a', '2026-09-28T10:00:00Z')])
    await late.push([])
    expect(items(late.n)).toEqual([])
    await late.push([])
    expect(late.fetchLedger).toHaveBeenCalledTimes(2)
    expect(items(late.n)[0]!.outcome).toBe('completed')

    const never = setup({ ledger: [[], []] })
    await never.push([auto('ws', 'a', '2026-09-28T10:00:00Z')])
    await never.push([])
    await never.push([])
    expect(items(never.n)[0]!.outcome).toBe('ended')
    await never.push([])
    expect(never.fetchLedger).toHaveBeenCalledTimes(2)
  })

  it('coalesces the runs that ended in one tick into one notification', async () => {
    const { n, push, announce } = setup()
    await push([chat('a', 't'), chat('b', 't'), chat('c', 't')])
    await push([])
    expect(n.notifications.value).toHaveLength(1)
    expect(n.notifications.value[0]!.items).toHaveLength(3)
    expect(announce).toHaveBeenCalledTimes(1)
    expect(announce).toHaveBeenCalledWith('3 runs ended')
  })

  it('keeps a bounded buffer, dropping the oldest', async () => {
    const { n, push } = setup({ cap: 2 })
    for (const t of ['t1', 't2', 't3']) {
      await push([chat('ws', t)])
      await push([])
    }
    expect(items(n).map((i) => i.id)).toEqual(['chat:ws@t2', 'chat:ws@t3'])
  })

  it('counts unread runs and clears the ones now on screen', async () => {
    let showing = ''
    const { n, push } = setup({ showing: (item) => item.workspace === showing })
    await push([chat('a', 't'), chat('b', 't')])
    await push([])
    expect(n.unreadCount.value).toBe(2)
    showing = 'a'
    n.clearShown()
    expect(n.unreadCount.value).toBe(1)
    n.dismiss(n.notifications.value[0]!.id)
    expect(n.unreadCount.value).toBe(0)
  })
})

// Phase 6: a long idle session must cost nothing per tick and keep every list
// bounded, however many runs come and go.
describe('run notifications over a long session', () => {
  it('does no work on idle ticks and stays bounded across thousands of runs', async () => {
    const { n, push, fetchLedger } = setup({ cap: 20 })
    await push([])
    for (let i = 0; i < 3000; i++) await push([]) // ~8 h of idle ticks at 10 s
    expect(fetchLedger).not.toHaveBeenCalled()
    expect(n.notifications.value).toHaveLength(0)

    for (let i = 0; i < 1000; i++) {
      await push([chat('ws', `2026-09-30T00:00:${String(i % 60).padStart(2, '0')}Z`)])
      await push([])
    }
    expect(items(n).length).toBeLessThanOrEqual(20)
  })
})
