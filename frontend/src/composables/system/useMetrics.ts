import { ref, shallowRef, onActivated, onDeactivated, onMounted, onUnmounted } from 'vue'
import { MetricsApiService } from '../../services/monitoring/metricsService'
import { DEFAULT_LOG_LEVEL, POLL_INTERVAL_MS } from '../../constants/api'
import type { MetricsSample, SystemMetrics } from '../../types/metrics'
import type { LogLevel } from '../../types/api'
import { useToast } from '../useToast'
import { errorMessage } from '../../utils/errors'

const metrics = ref<SystemMetrics | null>(null)
// Capped client history for sparklines (plan V8), filled by the existing poll —
// no extra timer. ~5 minutes at the 10 s interval.
const HISTORY_LIMIT = 30
const history = shallowRef<MetricsSample[]>([])
const logLevel = ref<LogLevel>(DEFAULT_LOG_LEVEL)

let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0 // track how many components are mounted to manage the poll lifecycle
let metricsReqId = 0 // request token to prevent stale responses
let logLevelFetched = false
let visibilityWatched = false

const refresh = async (): Promise<void> => {
  const mine = ++metricsReqId
  try {
    const data = await MetricsApiService.fetchMetrics()
    if (mine !== metricsReqId) return
    metrics.value = data
    history.value = [...history.value, { tokensPerSecond: data.llm_tokens_per_sec ?? 0, loadPercent: data.load_percent ?? 0 }].slice(-HISTORY_LIMIT)
  } catch (e: any) {
    if (mine !== metricsReqId) return
    console.error('[useMetrics] fetch metrics failed:', e.message)
  }
}

const startTimer = () => {
  if (!pollInterval) pollInterval = setInterval(refresh, POLL_INTERVAL_MS)
}

const stopTimer = () => {
  if (pollInterval) clearInterval(pollInterval)
  pollInterval = null
}

const tabHidden = () => typeof document !== 'undefined' && document.visibilityState === 'hidden'

// A hidden tab has nobody to show the figures to: the poll pauses and, on
// return, refreshes at once. Registered once for the app's lifetime, like
// useGlobalRunActivity; it only acts while a consumer holds the poll.
function watchVisibility() {
  if (visibilityWatched || typeof document === 'undefined') return
  visibilityWatched = true
  document.addEventListener('visibilitychange', () => {
    if (mountCount === 0) return
    if (tabHidden()) {
      stopTimer()
    } else if (!pollInterval) {
      void refresh()
      startTimer()
    }
  })
}

const fetchLogLevel = async (): Promise<void> => {
  try {
    logLevel.value = await MetricsApiService.fetchLogLevel()
  } catch (e: any) {
    console.error('[useMetrics] fetch log level failed:', e.message)
  }
}

const updateLogLevel = async (level: string): Promise<void> => {
  try {
    await MetricsApiService.updateLogLevel(level as LogLevel)
    logLevel.value = level as LogLevel
  } catch (e) {
    useToast().error(`Could not change the log level: ${errorMessage(e)}`)
  }
}

export function useMetrics() {
  // A consumer counts while it is on screen: a kept-alive view in the
  // background releases the shared poll (Phase 6 leak audit) and takes it back
  // — with an immediate refresh — when it returns. onActivated also fires right
  // after the first mount inside KeepAlive; `active` makes that a no-op.
  let active = false
  function acquire() {
    if (active) return
    active = true
    mountCount++
    if (mountCount === 1) {
      refresh()
      if (!logLevelFetched) {
        logLevelFetched = true
        fetchLogLevel()
      }
      watchVisibility()
      if (!tabHidden()) startTimer()
    }
  }
  function release() {
    if (!active) return
    active = false
    mountCount--
    if (mountCount === 0) stopTimer()
  }
  onMounted(acquire)
  onActivated(acquire)
  onDeactivated(release)
  onUnmounted(release)

  return {
    metrics,
    history,
    logLevel,
    refresh,
    updateLogLevel,
  }
}

export function useLogLevel() {
  onMounted(async () => {
    if (!logLevelFetched) {
      logLevelFetched = true
      await fetchLogLevel()
    }
  })

  return {
    logLevel,
    updateLogLevel,
  }
}
