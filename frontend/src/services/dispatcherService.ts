import type { Automation, DispatcherMetrics, TriggerResponse } from '../types/dispatcher'

const BASE_URL = '/admin/api/dispatcher'

export const DispatcherService = {
  async listAutomations(): Promise<Automation[]> {
    const res = await fetch(`${BASE_URL}/automations`)
    if (!res.ok) throw new Error('Failed to fetch automations')
    return res.json()
  },

  async triggerAutomation(workspace: string, automation: string): Promise<TriggerResponse> {
    const res = await fetch(`${BASE_URL}/trigger/${workspace}/${automation}`, {
      method: 'POST',
    })
    if (!res.ok) throw new Error('Failed to trigger automation')
    return res.json()
  },

  async getMetrics(): Promise<DispatcherMetrics> {
    const res = await fetch(`${BASE_URL}/metrics`)
    if (!res.ok) throw new Error('Failed to fetch dispatcher metrics')
    return res.json()
  },

  async listWorkspaces(): Promise<{id: string}[]> {
    const res = await fetch(`${BASE_URL}/workspaces`)
    if (!res.ok) throw new Error('Failed to fetch workspaces')
    return res.json()
  },

  async createWorkspace(id: string): Promise<void> {
    const res = await fetch(`${BASE_URL}/workspaces`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id })
    })
    if (!res.ok) throw new Error('Failed to create workspace')
  },

  async listWorkspaceFiles(workspace: string): Promise<string[]> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/files`)
    if (!res.ok) throw new Error('Failed to fetch workspace files')
    return res.json()
  },

  async readWorkspaceFile(workspace: string, file: string): Promise<string> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/files/${file}`)
    if (!res.ok) throw new Error('Failed to read file')
    const data = await res.json()
    return data.content
  },

  async writeWorkspaceFile(workspace: string, file: string, content: string): Promise<void> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/files/${file}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content })
    })
    if (!res.ok) throw new Error('Failed to write file')
  }
}
