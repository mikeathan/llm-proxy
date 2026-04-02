import { ref } from 'vue'
import type { Automation, DispatcherMetrics } from '../types/dispatcher'
import { DispatcherService } from '../services/dispatcherService'

const automations = ref<Automation[]>([])
const metrics = ref<DispatcherMetrics | null>(null)
const workspaces = ref<{id: string}[]>([])
const workspaceFiles = ref<Record<string, string[]>>({})
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

async function fetchWorkspaces() {
  try {
    workspaces.value = await DispatcherService.listWorkspaces()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch workspaces'
  }
}

async function fetchWorkspaceFiles(workspace: string) {
  try {
    const files = await DispatcherService.listWorkspaceFiles(workspace)
    workspaceFiles.value[workspace] = files
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch workspace files'
  }
}

async function createWorkspace(id: string) {
  await DispatcherService.createWorkspace(id)
  await fetchWorkspaces()
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
    workspaces,
    workspaceFiles,
    loading,
    error,
    fetchAutomations,
    fetchMetrics,
    triggerAutomation,
    fetchWorkspaces,
    fetchWorkspaceFiles,
    createWorkspace,
  }
}
