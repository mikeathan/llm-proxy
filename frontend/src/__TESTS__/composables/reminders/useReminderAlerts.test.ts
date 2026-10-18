import { describe, it, expect, vi } from 'vitest'
import { nextTick, ref, shallowRef } from 'vue'
import { createReminderAlerts } from '../../../composables/reminders/useReminderAlerts'
import { toReminders } from '../../../router/routes'
import type { GlobalRunTick } from '../../../types/assistant'
import type { ReminderAlertItem, ReminderEvent, ReminderEvents } from '../../../types/notifications'

const event = (id: number, at: number, kind: ReminderEvent['event'] = 'due', message = `reminder ${id}`): ReminderEvent =>
  ({ id, message, event: kind, at, channel: 'in-app' })

function setup(initialCursor: number | null = null) {
  const tick = shallowRef<GlobalRunTick | null>(null)
  const cursor = ref<number | null>(initialCursor)
  const replies: Array<ReminderEvents | Error> = []
  const fetchEvents = vi.fn(async (_after?: number) => {
    const next = replies.shift()
    if (next instanceof Error) throw next
    return next ?? { cursor: cursor.value ?? 0, items: [] }
  })
  const published: ReminderAlertItem[][] = []
  const alerts = createReminderAlerts({ lastTick: tick, fetchEvents, cursor, publish: (items) => published.push(items) })
  let seq = 0
  const poll = async (status: GlobalRunTick['status'] = 'ok') => {
    tick.value = { seq: ++seq, status, holders: [] }
    await nextTick()
    await alerts.settled()
  }
  return { poll, replies, fetchEvents, published, cursor }
}

describe('reminder alerts', () => {
  it('takes the first good poll as the baseline: nothing that fired before the app opened is announced', async () => {
    const { poll, replies, fetchEvents, published, cursor } = setup()
    replies.push({ cursor: 500, items: [] })
    await poll('error')
    expect(fetchEvents).not.toHaveBeenCalled()
    await poll()
    expect(fetchEvents).toHaveBeenCalledWith(undefined)
    expect(published).toEqual([])
    expect(cursor.value).toBe(500)
  })

  it('announces what fired after the cursor, oldest first, as one batch aimed at the Reminders page', async () => {
    const { poll, replies, fetchEvents, published, cursor } = setup(500)
    replies.push({ cursor: 900, items: [event(2, 900, 'failed'), event(1, 700)] })
    await poll()
    expect(fetchEvents).toHaveBeenCalledWith(500)
    expect(published).toHaveLength(1)
    expect(published[0]!.map((i) => i.event)).toEqual(['due', 'failed'])
    expect(published[0]![0]).toMatchObject({ kind: 'reminder', message: 'reminder 1', target: toReminders(), destination: 'reminders' })
    expect(cursor.value).toBe(900)
  })

  it('asks from the new cursor next time and does not announce the same outcome twice', async () => {
    const { poll, replies, fetchEvents, published } = setup(500)
    replies.push({ cursor: 900, items: [event(1, 900)] })
    await poll()
    replies.push({ cursor: 900, items: [event(1, 900)] })
    await poll()
    expect(fetchEvents).toHaveBeenLastCalledWith(900)
    expect(published).toHaveLength(1)
  })

  it('announces a recurring reminder again when it fires again', async () => {
    const { poll, replies, published } = setup(500)
    replies.push({ cursor: 900, items: [event(1, 900)] })
    await poll()
    replies.push({ cursor: 1900, items: [event(1, 1900)] })
    await poll()
    expect(published).toHaveLength(2)
  })

  it('keeps the cursor and tries again when the poll fails', async () => {
    const { poll, replies, published, cursor } = setup(500)
    replies.push(new Error('network'))
    await poll()
    expect(cursor.value).toBe(500)
    replies.push({ cursor: 900, items: [event(1, 900)] })
    await poll()
    expect(published).toHaveLength(1)
  })

  it('follows the server\'s newest event time even when it is older than the one kept, so a reset server is not ignored', async () => {
    const { poll, replies, published, cursor } = setup(9_000)
    replies.push({ cursor: 0, items: [] })
    await poll()
    expect(cursor.value).toBe(0)
    replies.push({ cursor: 400, items: [event(1, 400)] })
    await poll()
    expect(published).toHaveLength(1)
  })

  it('stays quiet when nothing fired', async () => {
    const { poll, replies, published, cursor } = setup(500)
    replies.push({ cursor: 500, items: [] })
    await poll()
    expect(published).toEqual([])
    expect(cursor.value).toBe(500)
  })

  it('never lets a slow poll interleave with the next', async () => {
    const tick = shallowRef<GlobalRunTick | null>(null)
    const cursor = ref<number | null>(500)
    let inFlight = 0
    let overlapped = false
    const fetchEvents = vi.fn(async () => {
      overlapped ||= ++inFlight > 1
      await new Promise((resolve) => setTimeout(resolve, 5))
      inFlight--
      return { cursor: 500, items: [] }
    })
    const alerts = createReminderAlerts({ lastTick: tick, fetchEvents, cursor, publish: () => {} })
    tick.value = { seq: 1, status: 'ok', holders: [] }
    await nextTick()
    tick.value = { seq: 2, status: 'ok', holders: [] }
    await nextTick()
    await alerts.settled()
    expect(fetchEvents).toHaveBeenCalledTimes(2)
    expect(overlapped).toBe(false)
  })
})
