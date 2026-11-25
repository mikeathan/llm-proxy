import { ref } from 'vue'
import { NewsroomService } from '../../services/newsroom/newsroomService'
import { sharedPoll } from '../ui/useSharedPoll'
import { ALERTS_UNREAD_POLL_MS } from '../../constants/newsroom'

// How many alerts wait unread (the Alerts tab's count, the Newsroom dot in the sidebar) and how many items the model left out (the switch's count), read by ONE poller. A server that cannot answer (the module
// off, the stack not running) shows no count and no error.
const unread = ref(0)
const leftOut = ref(0)

async function load(): Promise<void> {
  try {
    const counts = await NewsroomService.alertCounts()
    unread.value = counts.unread
    leftOut.value = counts.leftOut
  } catch {
    unread.value = 0
    leftOut.value = 0
  }
}
const poll = sharedPoll(load, ALERTS_UNREAD_POLL_MS)

/** The unread alerts for a component: shares one poller with every other component that asks. refresh() asks now, after a change. */
export function useAlertsUnread() {
  poll.use()
  return { unread, leftOut, refresh: poll.refresh }
}

/** Test seam: stops the poller and forgets the count. */
export function resetAlertsUnread(): void {
  poll.reset()
  unread.value = 0
  leftOut.value = 0
}
