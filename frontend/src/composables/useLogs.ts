import { ref, onMounted, onUnmounted } from 'vue'
import { MetricsApiService } from '../services/metricsService'
import { POLL_INTERVAL_MS } from '../constants/api'

// Shared singleton state
const processLogLines = ref<string>('')
const processLogRunning = ref(false)
const processLogName = ref('')
const processLogReady = ref(false)

const appLogLines = ref<string>('')
const appLogsFetched = ref(false)
const appLogsActive = ref(false) // Toggle to enable/disable app log polling (heavy)

let _wasRunning = false
let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0

async function tick() {
  // 1. Process logs (singleton poll)
  try {
    const data = await MetricsApiService.fetchLogs()
    processLogRunning.value = data.running
    processLogName.value = data.name ?? ''
    processLogReady.value = data.ready ?? false

    // Reset buffer on new model start
    if (data.running && !_wasRunning) {
      processLogLines.value = ''
    }
    _wasRunning = data.running

    if (data.logs) {
      processLogLines.value = data.logs
    }
  } catch (e: any) {
    console.error('[useLogs] process log poll failed:', e.message)
  }

  // 2. App logs (lazy singleton poll)
  if (appLogsActive.value) {
    try {
      const data = await MetricsApiService.fetchAppLogs()
      if (data.logs !== undefined) {
        appLogLines.value = data.logs
        appLogsFetched.value = true
      }
    } catch (e: any) {
      console.error('[useLogs] app log poll failed:', e.message)
    }
  }
}

export function useLogs() {
  onMounted(() => {
    mountCount++
    if (mountCount === 1) {
      tick()
      pollInterval = setInterval(tick, POLL_INTERVAL_MS)
    }
  })

  onUnmounted(() => {
    mountCount--
    if (mountCount === 0 && pollInterval) {
      clearInterval(pollInterval)
      pollInterval = null
    }
  })

  const clearProcessLogs = async (): Promise<void> => {
    try {
      await MetricsApiService.clearLogs()
      processLogLines.value = ''
    } catch (e: any) {
      console.error('[useLogs] clear process logs failed:', e.message)
    }
  }

  const clearAppLogs = async (): Promise<void> => {
    try {
      await MetricsApiService.clearAppLogs()
      appLogLines.value = ''
    } catch (e: any) {
      console.error('[useLogs] clear app logs failed:', e.message)
    }
  }

  return {
    // state
    processLogLines,
    processLogRunning,
    processLogName,
    processLogReady,
    appLogLines,
    appLogsFetched,
    appLogsActive,
    // actions
    clearProcessLogs,
    clearAppLogs,
    refresh: tick,
  }
}
