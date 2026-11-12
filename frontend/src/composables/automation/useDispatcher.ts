import { HEARTBEAT_TASK_FILE } from '../../utils/automation/heartbeat'
import { computed, markRaw, ref, shallowRef } from 'vue'
import type { Automation, AutomationRun, AgentState, DispatcherMetrics, TriggerResponse } from '../../types/dispatcher'
import { useAppBanner } from '../ui/useAppBanner'
import { DispatcherService } from '../../services/automation/dispatcherService'
import type { WorkspaceTree } from '../../types/workspace'
import type { AutomationPayload } from '../../types/automation'
import { taskFileChoices, withoutNestedPaths } from '../../utils/workspace/fileTree'

const { show: showBanner, clear: clearBanner } = useAppBanner()

const TRIGGER_QUEUED_STATUS = 'queued'
const TRIGGER_SKIPPED_STATUS = 'skipped'

const automations = ref<Automation[]>([])
const metrics = ref<DispatcherMetrics | null>(null)
const workspaces = ref<{ id: string }[]>([])
// One tree per workspace. shallowRef + markRaw: trees can hold thousands of
// entries and are replaced wholesale, never mutated (performance budget).
const workspaceTrees = shallowRef<Record<string, WorkspaceTree>>({})
// The files the automation task-file picker offers per workspace, derived from the
// tree — one listing, one derivation. What runs saved (reports/) is left out: it is a result, not a task.
const workspaceFiles = computed<Record<string, string[]>>(() =>
  Object.fromEntries(Object.entries(workspaceTrees.value).map(([ws, tree]) => [ws, taskFileChoices(tree)])),
)
const loading = ref(false)

// The page asks for the list again and again while a run is in flight, and a reply can arrive after a newer one: only the
// reply to the latest request is applied, so an older state (a run still shown as running) never replaces a newer one.
let latestAutomationsRequest = 0

async function fetchAutomations(silent = false) {
  if (!silent) loading.value = true
  clearBanner()
  const request = ++latestAutomationsRequest
  try {
    const list = await DispatcherService.listAutomations()
    if (request === latestAutomationsRequest) automations.value = list
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

async function fetchWorkspaceTree(workspace: string) {
  try {
    const tree = await DispatcherService.listWorkspaceTree(workspace)
    workspaceTrees.value = { ...workspaceTrees.value, [workspace]: markRaw(tree) }
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to fetch workspace files' })
    console.error('fetchWorkspaceTree error:', e)
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

async function triggerAutomation(workspace: string, automation: string, recordingRef?: string): Promise<TriggerResponse> {
  clearBanner()
  try {
    const result = await DispatcherService.triggerAutomation(workspace, automation, recordingRef)
    await fetchAutomations()
    // A queued trigger is a success the user must see: the run did not start,
    // it waits in the lane and executes when a slot frees.
    if (result.status === TRIGGER_QUEUED_STATUS) {
      showBanner({ severity: 'notice', message: `Queued #${result.position} — ${automation} starts when the lane frees` })
    }
    if (result.status === TRIGGER_SKIPPED_STATUS) {
      showBanner({ severity: 'notice', message: `${automation} was skipped: it has nothing to run yet. Add checks to ${HEARTBEAT_TASK_FILE}.` })
    }
    return result
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to trigger automation' })
    console.error('triggerAutomation error:', e)
    throw e
  }
}

async function cancelQueued(workspace: string, automation: string) {
  try {
    await DispatcherService.cancelQueued(workspace, automation)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to cancel queued automation' })
    console.error('cancelQueued error:', e)
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

async function updateAutomation(workspace: string, oldName: string, automation: AutomationPayload) {
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

/**
 * Deletes several automations, one request each. A failure does not stop the
 * rest; the failures are named in one banner and the list is refreshed once.
 * Returns the names that were deleted.
 */
async function deleteAutomations(targets: { workspace: string; name: string }[]): Promise<string[]> {
  const deleted: string[] = []
  const failed: string[] = []
  for (const { workspace, name } of targets) {
    try {
      await DispatcherService.deleteAutomation(workspace, name)
      deleted.push(name)
    } catch (e) {
      failed.push(`${name} (${e instanceof Error ? e.message : 'unknown error'})`)
      console.error('deleteAutomations error:', e)
    }
  }
  if (failed.length) {
    showBanner({ severity: 'error', message: `Could not delete ${failed.length} of ${targets.length}: ${failed.join(', ')}` })
  }
  await fetchAutomations()
  return deleted
}

async function createAutomation(workspace: string, automation: AutomationPayload) {
  try {
    await DispatcherService.createAutomation(workspace, automation)
    await fetchAutomations()
  } catch (e) {
    showBanner({ severity: 'error', message: e instanceof Error ? e.message : 'Failed to create automation' })
    console.error('createAutomation error:', e)
    throw e
  }
}

/**
 * Deletes files and folders (a folder with its content), one request each,
 * skipping paths a deleted folder takes along. A failure does not stop the
 * rest; the failures are named in one banner and the tree is refreshed once.
 * Returns the paths that were deleted.
 */
async function deleteWorkspacePaths(workspace: string, paths: string[]): Promise<string[]> {
  const targets = withoutNestedPaths(paths)
  const deleted: string[] = []
  const failed: string[] = []
  for (const path of targets) {
    try {
      await DispatcherService.deleteWorkspacePath(workspace, path)
      deleted.push(path)
    } catch (e) {
      failed.push(`${path} (${e instanceof Error ? e.message : 'unknown error'})`)
      console.error('deleteWorkspacePaths error:', e)
    }
  }
  if (failed.length) {
    showBanner({ severity: 'error', message: `Could not delete ${failed.length} of ${targets.length}: ${failed.join(', ')}` })
  }
  await fetchWorkspaceTree(workspace)
  return deleted
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

// Confirmation is handled by the UI (ConfirmDialog), not here, so these
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
    workspaceTrees,
    workspaceFiles,
    loading,
    fetchAutomations,
    fetchMetrics,
    triggerAutomation,
    fetchWorkspaces,
    fetchWorkspaceTree,
    fetchWorkspaceState,
    fetchGlobalActivity,
    createWorkspace,
    deleteWorkspacePaths,
    deleteWorkspace,
    deleteAutomations,
    createAutomation,
    deleteAutomation,
    updateAutomation,
    stopAutomation,
    cancelQueued,
    deleteRun,
    deleteAutomationRuns,
  }
}
