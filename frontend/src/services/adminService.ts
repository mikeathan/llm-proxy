import { API_ENDPOINTS } from '../constants/api'
import type { AdminState, APIKeyItem, GlobalConfig, ProcessListResponse, ProcessKillResponse } from '../types/admin'
import type { Model, ProviderModelInfo } from '../types/model'

async function handleResponse<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return {} as T
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as Record<string, string>
    throw new Error(err['error'] || res.statusText)
  }
  return res.json() as Promise<T>
}

async function get<T>(url: string): Promise<T> {
  const res = await fetch(url)
  return handleResponse<T>(res)
}

async function post<T>(url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  return handleResponse<T>(res)
}

async function put<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

async function del<T = void>(url: string): Promise<T> {
  const res = await fetch(url, { method: 'DELETE' })
  return handleResponse<T>(res)
}

export const AdminApiService = {
  fetchState: (): Promise<AdminState> =>
    get<AdminState>(`${API_ENDPOINTS.state}?available=1`),

  startModel: (name: string): Promise<void> =>
    post<void>(API_ENDPOINTS.start, { name }),

  stopModel: (): Promise<void> =>
    post<void>(API_ENDPOINTS.stop),

  addModel: (payload: Partial<Model>): Promise<Model> =>
    post<Model>(API_ENDPOINTS.models, payload),

  updateModel: (payload: Partial<Model>): Promise<Model> =>
    put<Model>(API_ENDPOINTS.models, payload),

  removeModel: (name: string): Promise<void> =>
    del(`${API_ENDPOINTS.models}?name=${encodeURIComponent(name)}`),

  removeAllModels: (provider: string): Promise<void> =>
    del(`${API_ENDPOINTS.modelsAll}?provider=${encodeURIComponent(provider)}`),

  updateConfig: (payload: Partial<GlobalConfig>): Promise<void> =>
    put<void>(API_ENDPOINTS.config, payload),
  
  fetchProviderModels: (provider: string, apiKeyName?: string): Promise<ProviderModelInfo[]> =>
    get<ProviderModelInfo[]>(`${API_ENDPOINTS.providerModels}?${new URLSearchParams({ provider, ...(apiKeyName ? { api_key_name: apiKeyName } : {}) })}`),

  fetchProviderManifests: (): Promise<any[]> =>
    get<any[]>(API_ENDPOINTS.providerManifests),

  testConnection: (provider: string, apiKey?: string, apiKeyName?: string, baseURL?: string): Promise<{ status: string; message: string }> =>
    get<{ status: string; message: string }>(`${API_ENDPOINTS.testConnection}?${new URLSearchParams({ provider, ...(apiKey ? { api_key: apiKey } : {}), ...(apiKeyName ? { api_key_name: apiKeyName } : {}), ...(baseURL ? { base_url: baseURL } : {}) })}`),

  fetchProviderKeys: (provider: string): Promise<APIKeyItem[]> =>
    get<APIKeyItem[]>(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}`),

  saveProviderKeys: (provider: string, keys: APIKeyItem[]): Promise<APIKeyItem[]> =>
    put<APIKeyItem[]>(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}`, keys),

  deleteProviderKey: (provider: string, keyId: string): Promise<void> =>
    del(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}&key_id=${encodeURIComponent(keyId)}`),

  deleteAllProviderKeys: (provider: string): Promise<APIKeyItem[]> =>
    del<APIKeyItem[]>(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}`),

  restartSystem: (): Promise<void> =>
    post<void>(API_ENDPOINTS.restart),
    
  fetchHostSettings: (): Promise<any> =>
    get<any>(API_ENDPOINTS.hostSettings),

  updateHostSettings: (payload: any): Promise<any> =>
    put<any>(API_ENDPOINTS.hostSettings, payload),

  resetTerminalSession: (workspaceID: string): Promise<void> =>
    post<void>(`${API_ENDPOINTS.terminalReset}?workspaceID=${encodeURIComponent(workspaceID)}`),

  fetchTerminalSessions: (): Promise<any[]> =>
    get<any[]>(API_ENDPOINTS.terminalSessions),

  fetchProcesses: (): Promise<ProcessListResponse> =>
    get<ProcessListResponse>(API_ENDPOINTS.processes),

  stopProcess: (pid: number): Promise<ProcessKillResponse> =>
    post<ProcessKillResponse>(`${API_ENDPOINTS.processes}/${pid}/stop`),
}
