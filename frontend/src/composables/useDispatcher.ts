import { ref } from 'vue'
import type { Automation, DispatcherMetrics, AgentState, AutomationRun } from '../types/dispatcher'

const automations = ref<Automation[]>([])
const metrics = ref<DispatcherMetrics | null>(null)
const workspaces = ref<{ id: string }[]>([])
const workspaceFiles = ref<Record<string, string[]>>({})
const loading = ref(false)
const error = ref<string | null>(null)

async function fetchAutomations() {
  loading.value = true
  error.value = null
  try {
    const res = await fetch('/admin/api/dispatcher/automations')
    const text = await res.text()
    if (!res.ok) {
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    automations.value = JSON.parse(text)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch automations'
    console.error('fetchAutomations error:', e)
  } finally {
    loading.value = false
  }
}

async function fetchWorkspaces() {
  try {
    const res = await fetch('/admin/api/dispatcher/workspaces')
    const text = await res.text()
    if (!res.ok) {
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    workspaces.value = JSON.parse(text)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch workspaces'
    console.error('fetchWorkspaces error:', e)
  }
}

async function fetchWorkspaceFiles(workspace: string) {
  try {
    const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}/files`)
    const text = await res.text()
    if (!res.ok) {
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    // Update ref object with new key in a reactive way
    workspaceFiles.value = { 
      ...workspaceFiles.value, 
      [workspace]: JSON.parse(text) 
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch workspace files'
    console.error('fetchWorkspaceFiles error:', e)
  }
}

async function fetchWorkspaceState(workspace: string): Promise<AgentState> {
  const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}/state`)
  const text = await res.text()
  if (!res.ok) {
    throw new Error(`Server error: ${res.status} - ${text}`)
  }
  return JSON.parse(text)
}

async function createWorkspace(id: string) {
  const res = await fetch('/admin/api/dispatcher/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`Server error: ${res.status} - ${text}`)
  }
  await fetchWorkspaces()
}

async function fetchMetrics() {
  try {
    const res = await fetch('/admin/api/dispatcher/metrics')
    const text = await res.text()
    if (!res.ok) {
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    metrics.value = JSON.parse(text)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch metrics'
    console.error('fetchMetrics error:', e)
  }
}

async function triggerAutomation(workspace: string, automation: string) {
  error.value = null
  try {
    const res = await fetch(`/admin/api/dispatcher/trigger/${workspace}/${automation}`, {
      method: 'POST'
    })
    const text = await res.text()
    if (!res.ok) {
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    // Refresh automations to get current output
    await fetchAutomations()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to trigger automation'
    console.error('triggerAutomation error:', e)
    throw e
  }
}

async function updateAutomation(workspace: string, oldName: string, automation: Automation) {
  try {
    const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}/automations/${oldName}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(automation)
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    await fetchAutomations()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update automation'
    console.error('updateAutomation error:', e)
    throw e
  }
}

function clearError() {
  error.value = null
}

async function deleteAutomation(workspace: string, automation: string) {
  try {
    const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}/automations/${automation}`, {
      method: 'DELETE'
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    await fetchAutomations()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete automation'
    console.error('deleteAutomation error:', e)
    throw e
  }
}

async function createAutomation(workspace: string, automation: Automation) {
  try {
    const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}/automations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(automation)
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    await fetchAutomations()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to create automation'
    console.error('createAutomation error:', e)
    throw e
  }
}

async function deleteWorkspaceFile(workspace: string, file: string) {
  try {
    const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}/files/${file}`, {
      method: 'DELETE'
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    await fetchWorkspaceFiles(workspace)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete file'
    console.error('deleteWorkspaceFile error:', e)
    throw e
  }
}

async function deleteWorkspace(workspace: string) {
  try {
    const res = await fetch(`/admin/api/dispatcher/workspaces/${workspace}`, {
      method: 'DELETE'
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Server error: ${res.status} - ${text}`)
    }
    await fetchWorkspaces()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete workspace'
    console.error('deleteWorkspace error:', e)
    throw e
  }
}

async function fetchGlobalActivity(): Promise<AutomationRun[]> {
  const res = await fetch('/admin/api/dispatcher/activity')
  const text = await res.text()
  if (!res.ok) {
    throw new Error(`Server error: ${res.status} - ${text}`)
  }
  return JSON.parse(text)
}

export function useDispatcher() {
  return {
    automations,
    metrics,
    workspaces,
    workspaceFiles,
    loading,
    error,
    clearError,
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
  }
}
