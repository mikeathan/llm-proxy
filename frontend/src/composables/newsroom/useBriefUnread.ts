import { computed, onMounted, onUnmounted, ref } from 'vue'
import { BriefService } from '../../services/newsroom/briefService'
import { BRIEF_UNREAD_POLL_MS } from '../../constants/newsroom'
import type { BriefLatest } from '../../types/newsroom'

// Which briefs wait unread: the newest edition of each brief and whether the owner read it. The state is the server's, so a dot is the same on every screen the
// owner uses. There is ONE poller however many places show a dot (the Newsroom tab, the dashboard's tab, the dashboard card): it runs while any of them is on
// screen and the page is visible. A server that cannot answer (the Briefs module off, the stack not running) shows nothing and no error: no brief, no dot.
const latest = ref<BriefLatest[]>([])
const loaded = ref(false)
let users = 0
let timer: ReturnType<typeof setInterval> | null = null

async function refresh(): Promise<void> {
  try {
    latest.value = await BriefService.latest()
  } catch {
    latest.value = []
  } finally {
    loaded.value = true
  }
}
const tick = () => {
  if (document.visibilityState !== 'hidden') void refresh()
}
const onVisible = () => {
  if (document.visibilityState === 'visible') void refresh()
}
function start() {
  void refresh()
  timer = setInterval(tick, BRIEF_UNREAD_POLL_MS)
  document.addEventListener('visibilitychange', onVisible)
}
function stop() {
  if (timer !== null) clearInterval(timer)
  timer = null
  document.removeEventListener('visibilitychange', onVisible)
}

/** Notes that the owner read an edition; the dot goes at once and the server keeps it. A refused request puts the dot back with the next refresh. */
async function markRead(issueId: number): Promise<void> {
  const entry = latest.value.find((l) => l.issue_id === issueId)
  if (entry?.read) return
  if (entry) latest.value = latest.value.map((l) => (l.issue_id === issueId ? { ...l, read: true } : l))
  try {
    await BriefService.markRead(issueId)
    // An edition the poll has not seen yet (just written by hand) is not in the list: ask, so no dot waits for the next tick.
    if (!entry) await refresh()
  } catch {
    void refresh()
  }
}

/** The unread briefs for a component: shares one poller with every other component that asks. */
export function useBriefUnread() {
  onMounted(() => {
    users++
    if (users === 1) start()
  })
  onUnmounted(() => {
    users = Math.max(0, users - 1)
    if (users === 0) stop()
  })
  const unreadCount = computed(() => latest.value.filter((l) => !l.read).length)
  const isUnread = (editionId: number): boolean => latest.value.some((l) => l.edition_id === editionId && !l.read)
  // A critical home fact of an edition not yet read: the page it belongs to (Radio's facts, else Network) gets a dot. It goes when the edition is read, so a fact
  // that has since cleared does not leave a dot behind until the next edition.
  const criticalPages = computed<Array<'radio' | 'network'>>(() => {
    const ids = latest.value.filter((l) => !l.read).flatMap((l) => l.critical ?? [])
    const pages: Array<'radio' | 'network'> = []
    if (ids.some((id) => id.startsWith('radio_'))) pages.push('radio')
    if (ids.some((id) => !id.startsWith('radio_'))) pages.push('network')
    return pages
  })
  return { latest, loaded, unreadCount, isUnread, markRead, refresh, criticalPages }
}

/** Test seam: forgets what was read from the server and stops the poller. */
export function resetBriefUnread(): void {
  stop()
  users = 0
  latest.value = []
  loaded.value = false
}
