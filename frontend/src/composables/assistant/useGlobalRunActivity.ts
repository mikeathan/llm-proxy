import { computed, ref } from 'vue'
import { AssistantService } from '../../services/assistant/assistantService'
import { POLL_INTERVAL_MS } from '../../constants/api'
import type { GlobalActiveRunsResponse, LaneHolder, QueuedRun } from '../../types/assistant'

// useGlobalRunActivity polls the workspace-independent active-runs endpoint so
// run visibility is not confined to the Agent IDE: the header indicator can show
// what is running or waiting in any workspace. The payload is the global
// run-scheduler snapshot (every run occupies a lane), so no workspace context is
// required.
//
// Implemented as a module-level singleton (like useAssistant and
// useRunningActivity) so the polling interval is shared and lives for the
// app's lifetime regardless of how many components consume it.
const laneHolders = ref<LaneHolder[]>([])
const queuedRuns = ref<QueuedRun[]>([])
const error = ref<string | null>(null)
let timer: ReturnType<typeof setInterval> | null = null

const runningCount = computed(() => laneHolders.value.length)
const queuedCount = computed(() => queuedRuns.value.length)
const isActive = computed(() => runningCount.value > 0 || queuedCount.value > 0)

async function refresh() {
  try {
    const data: GlobalActiveRunsResponse = await AssistantService.getGlobalActiveRuns()
    laneHolders.value = data.lane_holders ?? []
    queuedRuns.value = data.queued ?? []
    error.value = null
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load running activity'
  }
}

export function useGlobalRunActivity() {
  // Every consumer gets an immediate snapshot; the interval itself stays
  // singular so polling does not multiply with mounts.
  refresh()
  if (!timer) {
    timer = setInterval(refresh, POLL_INTERVAL_MS)
  }

  return { laneHolders, queuedRuns, runningCount, queuedCount, isActive, error, refresh }
}
