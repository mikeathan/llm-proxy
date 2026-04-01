import { ref } from 'vue'
import type { Workspace } from '../types/workspace'

// Global state
const workspaces = ref<Workspace[]>([])
const loading = ref(false)

// Fetch all workspaces
const fetchWorkspaces = async () => {
  loading.value = true
  try {
    const res = await fetch('/admin/api/workspaces')
    if (res.ok) {
      workspaces.value = await res.json() || []
    }
  } catch (err) {
    console.error('Failed to fetch workspaces', err)
  } finally {
    loading.value = false
  }
}

// Save a workspace
const saveWorkspace = async (workspace: Workspace) => {
  try {
    const res = await fetch('/admin/api/workspaces', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(workspace)
    })
    if (res.ok) {
      await fetchWorkspaces()
    } else {
      throw new Error(await res.text())
    }
  } catch (err) {
    console.error('Failed to save workspace', err)
    throw err
  }
}

// Delete a workspace
const deleteWorkspace = async (id: string) => {
  try {
    const res = await fetch(`/admin/api/workspaces?id=${encodeURIComponent(id)}`, {
      method: 'DELETE'
    })
    if (res.ok) {
      await fetchWorkspaces()
    } else {
      throw new Error(await res.text())
    }
  } catch (err) {
    console.error('Failed to delete workspace', err)
    throw err
  }
}

// Trigger manual run
const triggerHeartbeat = async (id: string) => {
  try {
    const res = await fetch(`/admin/api/workspaces/trigger?id=${encodeURIComponent(id)}`, {
      method: 'POST'
    })
    if (res.ok) {
      await fetchWorkspaces()
    } else {
      throw new Error(await res.text())
    }
  } catch (err) {
    console.error('Failed to trigger heartbeat', err)
    throw err
  }
}

export function useWorkspaces() {
  return {
    workspaces,
    loading,
    fetchWorkspaces,
    saveWorkspace,
    deleteWorkspace,
    triggerHeartbeat
  }
}
