import { ref, onMounted, onUnmounted } from 'vue'
import { MetricsApiService } from '../../services/monitoring/metricsService'
import { POLL_INTERVAL_MS } from '../../constants/api'

const processLogLines = ref<string>('')
const processLogRunning = ref(false)
const processLogName = ref('')
const processLogReady = ref(false)

const appLogLines = ref<string>('')
const appLogsFetched = ref(false)
const appLogsActive = ref(false)

const isFetching = ref(false) 

// Private internal state
let _wasRunning = false
let pollTimer: ReturnType<typeof setTimeout> | null = null
let subscriberCount = 0

/**
 * The core logic loop. 
 * Uses recursive setTimeout to prevent request overlapping.
 */
async function poll() {
  if (isFetching.value) return
  isFetching.value = true

  try {
    // 1. Process Logs
    const data = await MetricsApiService.fetchLogs()
    processLogRunning.value = data.running
    processLogName.value = data.name ?? ''
    processLogReady.value = data.ready ?? false

    if (data.running && !_wasRunning) {
      processLogLines.value = '' // Reset on new start
    }
    _wasRunning = data.running

    if (data.logs !== undefined) {
      processLogLines.value = data.logs
    }

    // 2. App Logs (Lazy)
    if (appLogsActive.value) {
      const appData = await MetricsApiService.fetchAppLogs()
      if (appData.logs !== undefined) {
        appLogLines.value = appData.logs
        appLogsFetched.value = true
      }
    }
  } catch (err: any) {
    console.error('[useLogs] Poll error:', err.message)
  } finally {
    isFetching.value = false
    // Schedule next poll only if we still have subscribers
    if (subscriberCount > 0) {
      pollTimer = setTimeout(poll, POLL_INTERVAL_MS)
    }
  }
}

export function useLogs() {
  onMounted(() => {
    subscriberCount++
    // Start polling only if this is the first component to mount
    if (subscriberCount === 1 && !pollTimer) {
      poll()
    }
  })

  onUnmounted(() => {
    subscriberCount--
    // Stop polling if no components are listening
    if (subscriberCount <= 0 && pollTimer) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
  })

  // Actions
  const clearProcessLogs = async () => {
    await MetricsApiService.clearLogs()
    processLogLines.value = ''
  }

  const clearAppLogs = async () => {
    await MetricsApiService.clearAppLogs()
    appLogLines.value = ''
  }

  return {
    // State
    processLogLines,
    processLogRunning,
    processLogName,
    processLogReady,
    appLogLines,
    appLogsFetched,
    appLogsActive,
    isFetching,
    // Actions
    clearProcessLogs,
    clearAppLogs,
    refresh: poll, // Manual trigger
  }
}