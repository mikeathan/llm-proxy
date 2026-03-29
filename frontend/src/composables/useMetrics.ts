import { ref, onMounted, onUnmounted } from 'vue'
import { MetricsApiService } from '../services/metricsService'
import { DEFAULT_LOG_LEVEL, POLL_INTERVAL_MS } from '../constants/api'
import type { SystemMetrics, ProcessLogs } from '../types/metrics'
import type { LogLevel } from '../constants/api'

const metrics = ref<SystemMetrics | null>(null)
const logs = ref<ProcessLogs | null>(null)
const logLevel = ref<LogLevel>(DEFAULT_LOG_LEVEL)

let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0 // track how many components are mounted to manage the poll lifecycle

const refresh = async (): Promise<void> => {
  try {
    const [m, l] = await Promise.all([
      MetricsApiService.fetchMetrics(),
      MetricsApiService.fetchLogs(),
    ])
    metrics.value = m
    logs.value = l
  } catch (e: any) {
    console.error('[useMetrics] fetch failed:', e.message)
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

// onMounted/onUnmounted here are called in the context of the component
// that calls useMetrics(), tracking the poll lifecycle correctly.
export function useMetrics() {
  onMounted(() => {
    mountCount++
    if (mountCount === 1) {
      // First consumer boots the poll
      refresh()
      fetchLogLevel()
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
    logs,
    logLevel,
    refresh,
    updateLogLevel,
  }
}
