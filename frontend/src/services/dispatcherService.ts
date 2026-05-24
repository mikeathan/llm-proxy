import type { Automation, DispatcherMetrics, TriggerResponse, RecordingMeta, RecordingStatus } from '../types/dispatcher'

const BASE_URL = '/admin/api/dispatcher'
const RECORDINGS_URL = '/admin/api/recordings'

export const DispatcherService = {
  async listAutomations(): Promise<Automation[]> {
    const res = await fetch(`${BASE_URL}/automations`)
    if (!res.ok) throw new Error('Failed to fetch automations')
    return res.json()
  },

  async triggerAutomation(workspace: string, automation: string, recordingRef?: string): Promise<TriggerResponse> {
    let url = `${BASE_URL}/trigger/${workspace}/${automation}`
    if (recordingRef) {
      url += `?recording_ref=${encodeURIComponent(recordingRef)}`
    }
    const res = await fetch(url, {
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
  },

  async deleteWorkspaceFile(workspace: string, file: string): Promise<void> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/files/${file}`, {
      method: 'DELETE'
    })
    if (!res.ok) throw new Error('Failed to delete file')
  },

  async deleteWorkspace(workspace: string): Promise<void> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}`, {
      method: 'DELETE'
    })
    if (!res.ok) throw new Error('Failed to delete workspace')
  },

  async createAutomation(workspace: string, automation: any): Promise<void> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/automations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(automation)
    })
    if (!res.ok) throw new Error('Failed to create automation')
  },

  async getWorkspaceConfig(workspace: string): Promise<any> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/config`)
    if (!res.ok) throw new Error('Failed to fetch workspace config')
    return res.json()
  },

  async updateWorkspaceConfig(workspace: string, config: any): Promise<void> {
    const res = await fetch(`${BASE_URL}/workspaces/${workspace}/config`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config)
    })
    if (!res.ok) throw new Error('Failed to update workspace config')
  },

  async getAllWorkspaceConfigs(): Promise<Record<string, any>> {
    const workspaces = await this.listWorkspaces()
    const configs: Record<string, any> = {}
    await Promise.all(workspaces.map(async (ws) => {
      try {
        configs[ws.id] = await this.getWorkspaceConfig(ws.id)
      } catch {
        // Skip workspaces that fail to load config
      }
    }))
    return configs
  },

  // --- Recordings ---

  async getRecordingStatus(): Promise<RecordingStatus> {
    const res = await fetch(`${RECORDINGS_URL}/status`)
    if (!res.ok) throw new Error('Failed to fetch recording status')
    return res.json()
  },

  async listRecordings(automation?: string): Promise<RecordingMeta[]> {
    const qs = automation ? `?automation=${encodeURIComponent(automation)}` : ''
    const res = await fetch(`${RECORDINGS_URL}${qs}`)
    if (!res.ok) throw new Error('Failed to fetch recordings')
    return res.json()
  },

  async getRecording(id: string): Promise<RecordingMeta> {
    const res = await fetch(`${RECORDINGS_URL}/${encodeURIComponent(id)}`)
    if (!res.ok) throw new Error('Recording not found')
    return res.json()
  },

  async deleteRecording(id: string): Promise<void> {
    const res = await fetch(`${RECORDINGS_URL}/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    })
    if (!res.ok) throw new Error('Failed to delete recording')
  },

  async setAutomationRecordingRef(workspace: string, automation: string, recordingRef: string): Promise<void> {
    // Get current automations from workspace config, update the ref, save
    const config = await this.getWorkspaceConfig(workspace)
    const auto = config.automations?.find((a: any) => a.name === automation)
    if (!auto) throw new Error('Automation not found in config')
    auto.recording_ref = recordingRef
    await this.updateWorkspaceConfig(workspace, config)
  },

  async clearAutomationRecordingRef(workspace: string, automation: string): Promise<void> {
    const config = await this.getWorkspaceConfig(workspace)
    const auto = config.automations?.find((a: any) => a.name === automation)
    if (!auto) throw new Error('Automation not found in config')
    delete auto.recording_ref
    await this.updateWorkspaceConfig(workspace, config)
  }
}
