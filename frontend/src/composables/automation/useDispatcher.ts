import { ref } from 'vue'
import type { Automation, AutomationRun, AgentState, DispatcherMetrics } from '../../types/dispatcher'
import { useAppBanner } from '../ui/useAppBanner'
import { DispatcherService } from '../../services/automation/dispatcherService'

const { show: showBanner, clear: clearBanner } = useAppBanner()

const automations = ref<Automation[]>([])
const metrics = ref<DispatcherMetrics | null>(null)
const workspaces = ref<{ id: string }[]>([])
const workspaceFiles = ref<Record<string, string[]>>({})
const loading = ref(false)

async function fetchAutomations(silent = false) {
  if (!silent) loading.value = true
  clearBanner()
  try {
    automations.value = await DispatcherService.listAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to fetch automations' })
    console.error('fetchAutomations error:', e)
  } finally {
    if (!silent) loading.value = false
  }
}

async function fetchWorkspaces() {
  try {
    workspaces.value = await DispatcherService.listWorkspaces()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to fetch workspaces' })
    console.error('fetchWorkspaces error:', e)
  }
}

async function fetchWorkspaceFiles(workspace: string) {
  try {
    const files = await DispatcherService.listWorkspaceFiles(workspace)
    workspaceFiles.value = {
      ...workspaceFiles.value,
      [workspace]: files,
    }
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to fetch workspace files' })
    console.error('fetchWorkspaceFiles error:', e)
  }
}

async function fetchWorkspaceState(workspace: string): Promise<AgentState> {
  return DispatcherService.getWorkspaceState(workspace)
}

async function createWorkspace(id: string) {
  try {
    await DispatcherService.createWorkspace(id)
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to create workspace' })
    console.error('createWorkspace error:', e)
    throw e
  }
  await fetchWorkspaces()
}

async function fetchMetrics() {
  try {
    metrics.value = await DispatcherService.getMetrics()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to fetch metrics' })
    console.error('fetchMetrics error:', e)
  }
}

async function triggerAutomation(workspace: string, automation: string, recordingRef?: string) {
  clearBanner()
  try {
    await DispatcherService.triggerAutomation(workspace, automation, recordingRef)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to trigger automation' })
    console.error('triggerAutomation error:', e)
    throw e
  }
}

async function stopAutomation(workspace: string) {
  clearBanner()
  try {
    await DispatcherService.stopAutomation(workspace)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to stop automation' })
    console.error('stopAutomation error:', e)
    throw e
  }
}

async function updateAutomation(workspace: string, oldName: string, automation: Automation) {
  try {
    await DispatcherService.updateAutomation(workspace, oldName, automation)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to update automation' })
    console.error('updateAutomation error:', e)
    throw e
  }
}

async function deleteAutomation(workspace: string, automation: string) {
  try {
    await DispatcherService.deleteAutomation(workspace, automation)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to delete automation' })
    console.error('deleteAutomation error:', e)
    throw e
  }
}

async function createAutomation(workspace: string, automation: Automation) {
  try {
    await DispatcherService.createAutomation(workspace, automation)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to create automation' })
    console.error('createAutomation error:', e)
    throw e
  }
}

async function deleteWorkspaceFile(workspace: string, file: string) {
  try {
    await DispatcherService.deleteWorkspaceFile(workspace, file)
    await fetchWorkspaceFiles(workspace)
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to delete file' })
    console.error('deleteWorkspaceFile error:', e)
    throw e
  }
}

async function deleteWorkspace(workspace: string) {
  try {
    await DispatcherService.deleteWorkspace(workspace)
    await fetchWorkspaces()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to delete workspace' })
    console.error('deleteWorkspace error:', e)
    throw e
  }
}

// Confirmation is handled by the UI (InlineConfirm), not here, so these
// functions perform the action directly and surface errors via a banner.
async function deleteRun(run: AutomationRun) {
  if (!run.workspace_id || !run.id) {
    return
  }
  try {
    await DispatcherService.deleteRun(run.workspace_id, run.id)
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to delete run' })
    console.error('deleteRun error:', e)
    throw e
  }
}

async function deleteAutomationRuns(workspace: string, automation: string) {
  if (!workspace || !automation) {
    return
  }
  try {
    await DispatcherService.deleteAutomationRuns(workspace, automation)
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to clear automation runs' })
    console.error('deleteAutomationRuns error:', e)
    throw e
  }
}

async function fetchGlobalActivity(): Promise<AutomationRun[]> {
  return DispatcherService.getGlobalActivity()
}

export function useDispatcher() {
  return {
    automations,
    metrics,
    workspaces,
    workspaceFiles,
    loading,
    fetchAutomations,
    fetchMetrics,
    triggerAutomation,
    fetchWorkspaces,
    fetchWorkspaceFiles,
    fetchWorkspaceState,
    fetchGlobalActivity,
    createWorkspace,
    deleteWorkspaceFile,
    deleteWorkspace,
    createAutomation,
    deleteAutomation,
    updateAutomation,
    stopAutomation,
    deleteRun,
    deleteAutomationRuns,
  }
}
