import type { Automation, AutomationRun, AgentState, DispatcherMetrics, TriggerResponse, RecordingMeta, RecordingStatus } from '../../types/dispatcher'
import { get, post, put, del } from '../httpClient'

const BASE_URL = '/admin/api/dispatcher'
const RECORDINGS_URL = '/admin/api/recordings'

export const DispatcherService = {
  async listAutomations(): Promise<Automation[]> {
    return get<Automation[]>(`${BASE_URL}/automations`)
  },

  async triggerAutomation(workspace: string, automation: string, recordingRef?: string): Promise<TriggerResponse> {
    let url = `${BASE_URL}/trigger/${workspace}/${automation}`
    if (recordingRef) {
      url += `?recording_ref=${encodeURIComponent(recordingRef)}`
    }
    return post<TriggerResponse>(url)
  },

  async getMetrics(): Promise<DispatcherMetrics> {
    return get<DispatcherMetrics>(`${BASE_URL}/metrics`)
  },

  async listWorkspaces(): Promise<{id: string}[]> {
    return get<{id: string}[]>(`${BASE_URL}/workspaces`)
  },

  async createWorkspace(id: string): Promise<void> {
    return post<void>(`${BASE_URL}/workspaces`, { id })
  },

  async listWorkspaceFiles(workspace: string): Promise<string[]> {
    return get<string[]>(`${BASE_URL}/workspaces/${workspace}/files`)
  },

  async readWorkspaceFile(workspace: string, file: string): Promise<string> {
    const data = await get<{ content: string }>(`${BASE_URL}/workspaces/${workspace}/files/${file}`)
    return data.content
  },

  async writeWorkspaceFile(workspace: string, file: string, content: string): Promise<void> {
    return put<void>(`${BASE_URL}/workspaces/${workspace}/files/${file}`, { content })
  },

  async deleteWorkspaceFile(workspace: string, file: string): Promise<void> {
    return del<void>(`${BASE_URL}/workspaces/${workspace}/files/${file}`)
  },

  async deleteWorkspace(workspace: string): Promise<void> {
    return del<void>(`${BASE_URL}/workspaces/${workspace}`)
  },

  async createAutomation(workspace: string, automation: Automation): Promise<void> {
    return post<void>(`${BASE_URL}/workspaces/${workspace}/automations`, automation)
  },

  async updateAutomation(workspace: string, oldName: string, automation: Automation): Promise<void> {
    return put<void>(`${BASE_URL}/workspaces/${workspace}/automations/${oldName}`, automation)
  },

  async deleteAutomation(workspace: string, automation: string): Promise<void> {
    return del<void>(`${BASE_URL}/workspaces/${workspace}/automations/${automation}`)
  },

  async stopAutomation(workspace: string): Promise<void> {
    return post<void>(`${BASE_URL}/stop/${workspace}`)
  },

  async getWorkspaceState(workspace: string): Promise<AgentState> {
    return get<AgentState>(`${BASE_URL}/workspaces/${workspace}/state`)
  },

  async getGlobalActivity(): Promise<AutomationRun[]> {
    return get<AutomationRun[]>(`${BASE_URL}/activity`)
  },

  async getWorkspaceConfig(workspace: string): Promise<any> {
    return get<any>(`${BASE_URL}/workspaces/${workspace}/config`)
  },

  async updateWorkspaceConfig(workspace: string, config: any): Promise<void> {
    return put<void>(`${BASE_URL}/workspaces/${workspace}/config`, config)
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
    return get<RecordingStatus>(`${RECORDINGS_URL}/status`)
  },

  async listRecordings(automation?: string): Promise<RecordingMeta[]> {
    const qs = automation ? `?automation=${encodeURIComponent(automation)}` : ''
    return get<RecordingMeta[]>(`${RECORDINGS_URL}${qs}`)
  },

  async getRecording(id: string): Promise<RecordingMeta> {
    return get<RecordingMeta>(`${RECORDINGS_URL}/${encodeURIComponent(id)}`)
  },

  async deleteRecording(id: string): Promise<void> {
    return del<void>(`${RECORDINGS_URL}/${encodeURIComponent(id)}`)
  },

  // Delete a single automation run (and its on-disk run directory when one
  // exists) by history ID so individual runs can be pruned from the UI.
  async deleteRun(workspace: string, runID: string): Promise<void> {
    const base = `/admin/api/dispatcher/runs/${encodeURIComponent(workspace)}/run/${encodeURIComponent(runID)}`
    return del<void>(base)
  },

  // Delete every run directory for an automation across all model subdirs and
  // purge the matching history from state.json (the "clear all runs" action).
  async deleteAutomationRuns(workspace: string, automation: string): Promise<void> {
    const base = `/admin/api/dispatcher/runs/${encodeURIComponent(workspace)}/${encodeURIComponent(automation)}`
    return del<void>(base)
  },

  async setAutomationRecordingRef(workspace: string, automation: string, recordingRef: string): Promise<void> {
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
