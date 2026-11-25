import { ref } from 'vue'
import { sharedPoll } from '../ui/useSharedPoll'
import { ModelStatsService } from '../../services/modelStatsService'
import { MODEL_USE_DAYS, MODEL_USE_POLL_MS } from '../../constants/modelStats'
import type { ModelStatsReport } from '../../types/modelStats'

// Shared by every Model use block on screen (the brief and a dashboard card): one request a minute however many show it.
const report = ref<ModelStatsReport | null>(null)
const failed = ref(false)

async function load() {
  try {
    report.value = await ModelStatsService.get(MODEL_USE_DAYS)
    failed.value = false
  } catch {
    // A server that cannot answer after it once did keeps the last numbers; one that never answered is said.
    failed.value = report.value === null
  }
}
const poll = sharedPoll(load, MODEL_USE_POLL_MS)

/** How much each model was used, read live (ADR-0085). */
export function useModelUse() {
  poll.use()
  return { report, failed }
}

/** Test seam: forgets the report and stops the timer. */
export function resetModelUse(): void {
  poll.reset()
  report.value = null
  failed.value = false
}
