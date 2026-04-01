import { ref } from 'vue'
import type { Automation, DispatcherMetrics } from '../types/dispatcher'
import { DispatcherService } from '../services/dispatcherService'

const automations = ref<Automation[]>([])
const metrics = ref<DispatcherMetrics | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

async function fetchAutomations() {
  loading.value = true
  error.value = null
  try {
    automations.value = await DispatcherService.listAutomations()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch automations'
  } finally {
    loading.value = false
  }
}

async function fetchMetrics() {
  try {
    metrics.value = await DispatcherService.getMetrics()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch metrics'
  }
}

async function triggerAutomation(workspace: string, automation: string) {
  error.value = null
  try {
    await DispatcherService.triggerAutomation(workspace, automation)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to trigger automation'
    throw e
  }
}

export function useDispatcher() {
  return {
    automations,
    metrics,
    loading,
    error,
    fetchAutomations,
    fetchMetrics,
    triggerAutomation,
  }
}
