import { watch, type Ref } from 'vue'
import { toReminders } from '../../router/routes'
import { ReminderService } from '../../services/reminders/reminderService'
import type { GlobalRunTick } from '../../types/assistant'
import type { ReminderAlertItem, ReminderEvent, ReminderEvents } from '../../types/notifications'
import { useGlobalRunActivity } from '../assistant/useGlobalRunActivity'
import { usePersistedState } from '../ui/usePersistedState'

// Tells the bell when a reminder fires (or fails to), whichever channels it was sent to. The server saves each outcome on
// the reminder with a time; this asks "what came after the newest I have seen" on the existing run poll (no second timer,
// paused with the tab, as run notifications are). The first good poll only learns where "now" is, so opening the app
// announces what fires from then on; a cursor kept from an earlier visit announces what fired while the app was closed.

// Remembered outcomes, so one is announced at most once. Bounded.
const SEEN_LIMIT = 100

interface Deps {
  lastTick: Readonly<Ref<GlobalRunTick | null>>
  /** The outcomes after a time; with none given, only the newest event time. */
  fetchEvents: (after?: number) => Promise<ReminderEvents>
  /** The newest event time seen, kept across visits; null until the first poll. */
  cursor: Ref<number | null>
  /** Puts a batch of alerts on the bell, oldest first. */
  publish: (items: ReminderAlertItem[]) => void
}

function alertOf(event: ReminderEvent): ReminderAlertItem {
  return {
    id: `reminder-${event.id}-${event.at}`,
    kind: 'reminder',
    message: event.message,
    event: event.event,
    channel: event.channel,
    target: toReminders(),
    destination: 'reminders',
  }
}

export function createReminderAlerts(deps: Deps) {
  const seen: string[] = []
  let queue: Promise<void> = Promise.resolve()

  function remember(id: string): boolean {
    if (seen.includes(id)) return false
    seen.push(id)
    if (seen.length > SEEN_LIMIT) seen.shift()
    return true
  }

  async function processTick(tick: GlobalRunTick) {
    if (tick.status !== 'ok') return
    const after = deps.cursor.value ?? undefined
    let reply: ReminderEvents
    try {
      reply = await deps.fetchEvents(after)
    } catch {
      // Treated as "nothing yet": the cursor stays, so the next poll asks for the same range.
      return
    }
    deps.cursor.value = reply.cursor
    if (after === undefined) return
    const fresh = reply.items.map(alertOf).filter((item) => remember(item.id)).reverse()
    if (fresh.length) deps.publish(fresh)
  }

  const stop = watch(deps.lastTick, (tick) => {
    if (!tick) return
    // Serialised: a slow read never interleaves with the next tick.
    queue = queue.then(() => processTick(tick))
  })

  return { stop, settled: () => queue }
}

const CURSOR_KEY = 'reminders.alert-cursor'
const CURSOR_VERSION = 1

let started = false

/**
 * Starts the app-wide reminder alerts once (called from main.ts, the app bootstrap): they ride the existing run poll and
 * put what fired on the same bell as the runs that ended. The cursor is kept across visits.
 */
export function startReminderAlerts(notifications: { publishReminders: (items: ReminderAlertItem[]) => void }) {
  if (started) return
  started = true
  const { lastTick } = useGlobalRunActivity()
  const cursor = usePersistedState<number | null>(CURSOR_KEY, {
    version: CURSOR_VERSION,
    fallback: null,
    parse: (value) => (typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null),
  })
  createReminderAlerts({
    lastTick,
    fetchEvents: (after) => ReminderService.events(after),
    cursor,
    publish: notifications.publishReminders,
  })
}
