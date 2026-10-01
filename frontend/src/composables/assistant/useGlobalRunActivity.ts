import { computed, ref, shallowRef } from 'vue'
import { AssistantService } from '../../services/assistant/assistantService'
import { POLL_INTERVAL_MS } from '../../constants/api'
import type { GlobalActiveRunsResponse, GlobalRunTick, LaneHolder, LaneSummary, QueuedRun } from '../../types/assistant'

// useGlobalRunActivity polls the workspace-independent active-runs endpoint so
// run visibility is not confined to the Agent IDE: the header indicator can show
// what is running or waiting in any workspace. The payload is the global
// run-scheduler snapshot (every run occupies a lane), so no workspace context is
// required.
//
// Implemented as a module-level singleton (like useAssistant and
// useRunningActivity) so the polling interval is shared and lives for the
// app's lifetime regardless of how many components consume it. The run
// notifications (useRunNotifications) read the same tick through lastTick —
// no second poller (plan D5). Polling pauses while the tab is hidden and
// ticks immediately when it is shown again (D20).
const laneHolders = ref<LaneHolder[]>([])
const queuedRuns = ref<QueuedRun[]>([])
const lanes = ref<LaneSummary[]>([])
const error = ref<string | null>(null)
const lastTick = shallowRef<GlobalRunTick | null>(null)
let tickSeq = 0
let timer: ReturnType<typeof setInterval> | null = null
let visibilityWatched = false

const runningCount = computed(() => laneHolders.value.length)
const queuedCount = computed(() => queuedRuns.value.length)
const isActive = computed(() => runningCount.value > 0 || queuedCount.value > 0)

async function refresh() {
  try {
    const data: GlobalActiveRunsResponse = await AssistantService.getGlobalActiveRuns()
    laneHolders.value = data.lane_holders ?? []
    queuedRuns.value = data.queued ?? []
    lanes.value = data.lanes ?? []
    error.value = null
    lastTick.value = { seq: ++tickSeq, status: 'ok', holders: laneHolders.value }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load running activity'
    lastTick.value = { seq: ++tickSeq, status: 'error', holders: laneHolders.value }
  }
}

function startTimer() {
  if (!timer) timer = setInterval(refresh, POLL_INTERVAL_MS)
}

function stopTimer() {
  if (timer) clearInterval(timer)
  timer = null
}

// Registered once for the app's lifetime, like the interval it controls.
function watchVisibility() {
  if (visibilityWatched || typeof document === 'undefined') return
  visibilityWatched = true
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      stopTimer()
    } else if (!timer) {
      void refresh()
      startTimer()
    }
  })
}

export function useGlobalRunActivity() {
  // Every consumer gets an immediate snapshot; the interval itself stays
  // singular so polling does not multiply with mounts.
  refresh()
  watchVisibility()
  if (typeof document === 'undefined' || document.visibilityState !== 'hidden') startTimer()

  return { laneHolders, queuedRuns, lanes, runningCount, queuedCount, isActive, error, lastTick, refresh }
}
