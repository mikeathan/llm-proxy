import { ref, onMounted, onUnmounted } from 'vue'
import { MetricsApiService } from '../../services/monitoring/metricsService'
import { DEFAULT_LOG_LEVEL, POLL_INTERVAL_MS } from '../../constants/api'
import type { SystemMetrics } from '../../types/metrics'
import type { LogLevel } from '../../types/api'

const metrics = ref<SystemMetrics | null>(null)
const logLevel = ref<LogLevel>(DEFAULT_LOG_LEVEL)

let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0 // track how many components are mounted to manage the poll lifecycle
let metricsReqId = 0 // request token to prevent stale responses
let logLevelFetched = false

const refresh = async (): Promise<void> => {
  const mine = ++metricsReqId
  try {
    const data = await MetricsApiService.fetchMetrics()
    if (mine !== metricsReqId) return
    metrics.value = data
  } catch (e: any) {
    if (mine !== metricsReqId) return
    console.error('[useMetrics] fetch metrics failed:', e.message)
  }
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
  } catch (e: any) {
    console.error(e)
    alert(`Error updating log level: ${e.message}`)
  }
}

export function useMetrics() {
  onMounted(() => {
    mountCount++
    if (mountCount === 1) {
      refresh()
      if (!logLevelFetched) {
        logLevelFetched = true
        fetchLogLevel()
      }
      pollInterval = setInterval(refresh, POLL_INTERVAL_MS)
    }
  })

  onUnmounted(() => {
    mountCount--
    if (mountCount === 0 && pollInterval) {
      clearInterval(pollInterval)
      pollInterval = null
    }
  })

  return {
    metrics,
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
