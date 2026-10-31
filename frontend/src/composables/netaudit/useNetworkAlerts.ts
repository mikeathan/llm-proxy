import { watch, type Ref } from 'vue'
import { toNetwork } from '../../router/routes'
import { NetauditService } from '../../services/netaudit/netauditService'
import type { GlobalRunTick } from '../../types/assistant'
import type { NetworkAlertEvent, NetworkAlertEvents, NetworkAlertItem } from '../../types/notifications'
import { useGlobalRunActivity } from '../assistant/useGlobalRunActivity'
import { useBrowserAlerts } from './useBrowserAlerts'
import { usePersistedState } from '../ui/usePersistedState'

// Tells the bell what the Network module alerted about (a device that connected to a decoy, a new device, the weekly
// summary), whether or not a connector exists. The server keeps a small log; this asks "what came after the newest I have
// seen" on the existing run poll (no second timer, paused with the tab). The first good poll only learns where "now" is, so
// opening the app announces what is raised from then on; a cursor kept from an earlier visit announces what was raised
// while the app was closed.

// Remembered entries, so one is announced at most once. Bounded.
const SEEN_LIMIT = 100

// Where each kind is looked at: devices and decoys live on the Devices tab; the summary is the overview.
const DEVICES_TAB = 'devices'

interface Deps {
  lastTick: Readonly<Ref<GlobalRunTick | null>>
  /** The alerts after an id; with none given, only the newest id. */
  fetchEvents: (after?: number) => Promise<NetworkAlertEvents>
  /** The newest alert id seen, kept across visits; null until the first poll. */
  cursor: Ref<number | null>
  /** Puts a batch of alerts on the bell, oldest first. */
  publish: (items: NetworkAlertItem[]) => void
}

function alertOf(event: NetworkAlertEvent): NetworkAlertItem {
  return {
    id: `network-${event.id}`,
    kind: 'network',
    alertKind: event.kind,
    message: event.message,
    delivery: event.delivery,
    count: event.count,
    repeatKey: `${event.kind}:${event.message}`,
    target: event.kind === 'summary' ? toNetwork() : toNetwork(DEVICES_TAB),
    destination: 'network',
  }
}

export function createNetworkAlerts(deps: Deps) {
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
    let reply: NetworkAlertEvents
    try {
      reply = await deps.fetchEvents(after)
    } catch {
      // Treated as "nothing yet": the cursor stays, so the next poll asks for the same range.
      return
    }
    // Anything but a log (a proxy's page, an older server) is treated as "nothing yet".
    if (typeof reply?.cursor !== 'number' || !Array.isArray(reply.items)) return
    deps.cursor.value = reply.cursor
    if (after === undefined) return
    const fresh = reply.items.map(alertOf).filter((item) => remember(item.id)).reverse()
    if (fresh.length) deps.publish(fresh)
  }

  const stop = watch(deps.lastTick, (tick) => {
    if (!tick) return
    // Serialised: a slow read never interleaves with the next tick. A failure in one tick never stops the ones after it.
    queue = queue.then(() => processTick(tick)).catch(() => {})
  })

  return { stop, settled: () => queue }
}

const CURSOR_KEY = 'netaudit.alert-cursor'
const CURSOR_VERSION = 1

let started = false

/**
 * Starts the app-wide Network alerts once (called from main.ts, the app bootstrap): they ride the existing run poll and put
 * what was raised on the same bell as the runs that ended. The cursor is kept across visits.
 */
export function startNetworkAlerts(notifications: { publishNetworkAlerts: (items: NetworkAlertItem[]) => void }) {
  if (started) return
  started = true
  const { lastTick } = useGlobalRunActivity()
  const browser = useBrowserAlerts()
  const cursor = usePersistedState<number | null>(CURSOR_KEY, {
    version: CURSOR_VERSION,
    fallback: null,
    parse: (value) => (typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null),
  })
  createNetworkAlerts({
    lastTick,
    fetchEvents: (after) => NetauditService.alerts(after),
    cursor,
    publish: (items) => {
      notifications.publishNetworkAlerts(items)
      browser.announce(items)
    },
  })
}
