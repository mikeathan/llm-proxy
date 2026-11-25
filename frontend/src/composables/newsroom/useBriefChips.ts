import { computed, ref } from 'vue'
import type { BriefChip } from '../../types/newsroom'
import { sharedPoll } from '../ui/useSharedPoll'
import { DispatcherService } from '../../services/automation/dispatcherService'
import { NetauditService } from '../../services/netaudit/netauditService'
import { BRIEF_CHIPS_WINDOW_MS, briefBlockedWords, briefFailedRunsWords } from '../../constants/newsroom'
import { toAutomations, toSettings } from '../../router/routes'
import { runOutcome } from '../../utils/automation/runOutcome'

const REFRESH_MS = 60000

// Shared by every place that shows the chips (the Brief card, the Today card, the brief): one pair of requests a minute however many show them.
const failedRuns = ref(0)
const blocked = ref(0)

async function load() {
  const since = Date.now() - BRIEF_CHIPS_WINDOW_MS
  const [runs, alerts] = await Promise.allSettled([DispatcherService.getGlobalActivity(), NetauditService.alerts(0)])
  failedRuns.value = runs.status === 'fulfilled' ? runs.value.filter((r) => Date.parse(r.timestamp) >= since && runOutcome(r) === 'failed').length : 0
  blocked.value = alerts.status === 'fulfilled' ? alerts.value.items.filter((a) => a.at >= since && (a.delivery === 'no_connector' || a.delivery === 'failed')).length : 0
}
const poll = sharedPoll(load, REFRESH_MS)

/**
 * What needs a look outside the brief's own facts, read live from what the service already keeps (like Model use, it is not part of the written edition): the
 * automation runs that failed in the last day, and the alerts that could not be sent because no connector took them. A chip shows only for a count above zero and links to the
 * page where it is fixed. A source that cannot be read gives no chip and no error: a missing chip must not be mistaken for a problem of its own.
 */
export function useBriefChips() {
  poll.use()
  const chips = computed<BriefChip[]>(() => [
    ...(failedRuns.value > 0 ? [{ key: 'failed_runs' as const, label: briefFailedRunsWords(failedRuns.value), to: toAutomations() }] : []),
    ...(blocked.value > 0 ? [{ key: 'blocked' as const, label: briefBlockedWords(blocked.value), to: toSettings('communication') }] : []),
  ])
  return { chips }
}

/** Test seam: forgets the counts and stops the timer. */
export function resetBriefChips(): void {
  poll.reset()
  failedRuns.value = 0
  blocked.value = 0
}
