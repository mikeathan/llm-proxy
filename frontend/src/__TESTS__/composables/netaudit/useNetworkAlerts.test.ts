import { describe, it, expect, vi } from 'vitest'
import { nextTick, ref, shallowRef } from 'vue'
import { createNetworkAlerts } from '../../../composables/netaudit/useNetworkAlerts'
import { toNetwork } from '../../../router/routes'
import type { GlobalRunTick } from '../../../types/assistant'
import type { NetworkAlertEvent, NetworkAlertEvents, NetworkAlertItem } from '../../../types/notifications'

const alert = (id: number, over: Partial<NetworkAlertEvent> = {}): NetworkAlertEvent =>
  ({ id, kind: 'presence', message: `alert ${id}`, delivery: 'sent', count: 1, at: id, ...over })

function setup(initialCursor: number | null = null, publishFails = false) {
  const tick = shallowRef<GlobalRunTick | null>(null)
  const cursor = ref<number | null>(initialCursor)
  const replies: Array<NetworkAlertEvents | Error> = []
  const fetchEvents = vi.fn(async (_after?: number) => {
    const next = replies.shift()
    if (next instanceof Error) throw next
    return next ?? { cursor: cursor.value ?? 0, items: [] }
  })
  const published: NetworkAlertItem[][] = []
  const alerts = createNetworkAlerts({
    lastTick: tick,
    fetchEvents,
    cursor,
    publish: (items) => {
      if (publishFails && published.length === 0) {
        published.push([])
        throw new Error('publish failed')
      }
      published.push(items)
    },
  })
  let seq = 0
  const poll = async (status: GlobalRunTick['status'] = 'ok') => {
    tick.value = { seq: ++seq, status, holders: [] }
    await nextTick()
    await alerts.settled()
  }
  return { poll, replies, fetchEvents, published, cursor }
}

describe('network alerts', () => {
  it('takes the first good poll as the baseline: nothing raised before the app opened is announced', async () => {
    const { poll, replies, fetchEvents, published, cursor } = setup()
    replies.push({ cursor: 500, items: [] })
    await poll('error')
    expect(fetchEvents).not.toHaveBeenCalled()
    await poll()
    expect(fetchEvents).toHaveBeenCalledWith(undefined)
    expect(published).toEqual([])
    expect(cursor.value).toBe(500)
  })

  it('announces what was raised after the cursor, oldest first, aimed at the Network page', async () => {
    const { poll, replies, published, cursor } = setup(500)
    replies.push({ cursor: 900, items: [alert(900, { kind: 'decoy', message: 'decoy m' }), alert(700, { kind: 'summary' })] })
    await poll()
    expect(published).toHaveLength(1)
    expect(published[0]!.map((i) => i.alertKind)).toEqual(['summary', 'decoy'])
    expect(published[0]![1]).toMatchObject({ kind: 'network', message: 'decoy m', repeatKey: 'decoy:decoy m', destination: 'network', target: toNetwork('devices') })
    expect(published[0]![0]!.target).toEqual(toNetwork())
    expect(cursor.value).toBe(900)
  })

  it('carries what became of the send and how often a decoy alert repeated', async () => {
    const { poll, replies, published } = setup(1)
    replies.push({ cursor: 5, items: [alert(5, { kind: 'decoy', delivery: 'no_connector', count: 3 })] })
    await poll()
    expect(published[0]![0]).toMatchObject({ delivery: 'no_connector', count: 3 })
  })

  it('does not announce the same entry twice, but an updated entry (a new id) again', async () => {
    const { poll, replies, fetchEvents, published } = setup(1)
    replies.push({ cursor: 5, items: [alert(5)] })
    await poll()
    replies.push({ cursor: 5, items: [alert(5)] })
    await poll()
    expect(fetchEvents).toHaveBeenLastCalledWith(5)
    expect(published).toHaveLength(1)
    replies.push({ cursor: 9, items: [alert(9)] })
    await poll()
    expect(published).toHaveLength(2)
  })

  it('keeps the cursor and tries again when the poll fails', async () => {
    const { poll, replies, published, cursor } = setup(500)
    replies.push(new Error('network'))
    await poll()
    expect(cursor.value).toBe(500)
    replies.push({ cursor: 900, items: [alert(900)] })
    await poll()
    expect(published).toHaveLength(1)
  })

  it('follows the server\'s cursor even when it is lower than the one kept, so a reset log is not ignored', async () => {
    const { poll, replies, published, cursor } = setup(9_000)
    replies.push({ cursor: 0, items: [] })
    await poll()
    expect(cursor.value).toBe(0)
    replies.push({ cursor: 400, items: [alert(400)] })
    await poll()
    expect(published).toHaveLength(1)
  })

  it('ignores an answer that is not an alert log, and keeps polling afterwards', async () => {
    const { poll, replies, published, cursor } = setup(500)
    replies.push({} as NetworkAlertEvents)
    await poll()
    expect(cursor.value).toBe(500)
    replies.push({ cursor: 900, items: [alert(900)] })
    await poll()
    expect(published).toHaveLength(1)
  })

  it('keeps announcing after one batch could not be published', async () => {
    const { poll, replies, published } = setup(500, true)
    replies.push({ cursor: 600, items: [alert(600)] })
    await poll()
    replies.push({ cursor: 700, items: [alert(700)] })
    await poll()
    expect(published[1]!.map((i) => i.id)).toEqual(['network-700'])
  })
})
