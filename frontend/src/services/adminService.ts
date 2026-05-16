import { API_ENDPOINTS } from '../constants/api'
import type { AdminState } from '../types/admin'
import type { Model } from '../types/model'
import type { GlobalConfig } from '../types/admin'

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
  
  fetchProviderModels: (provider: string, apiKeyName?: string): Promise<import('../types/model').ProviderModelInfo[]> => {
    const params = new URLSearchParams({ provider })
    if (apiKeyName) params.set('api_key_name', apiKeyName)
    return get<import('../types/model').ProviderModelInfo[]>(`${API_ENDPOINTS.providerModels}?${params.toString()}`)
  },

  fetchProviderManifests: (): Promise<any[]> =>
    get<any[]>(API_ENDPOINTS.providerManifests),

  testConnection: (provider: string, apiKey?: string, apiKeyName?: string, baseURL?: string): Promise<{ status: string; message: string }> => {
    const params = new URLSearchParams({ provider })
    if (apiKey) params.set('api_key', apiKey)
    if (apiKeyName) params.set('api_key_name', apiKeyName)
    if (baseURL) params.set('base_url', baseURL)
    return get<{ status: string; message: string }>(`${API_ENDPOINTS.testConnection}?${params.toString()}`)
  },

  fetchProviderKeys: (provider: string): Promise<import('../types/admin').APIKeyItem[]> => {
    return get(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}`)
  },

  saveProviderKeys: (provider: string, keys: import('../types/admin').APIKeyItem[]): Promise<import('../types/admin').APIKeyItem[]> => {
    return put(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}`, keys)
  },

  deleteProviderKey: (provider: string, keyId: string): Promise<void> =>
    del(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}&key_id=${encodeURIComponent(keyId)}`),

  deleteAllProviderKeys: (provider: string): Promise<import('../types/admin').APIKeyItem[]> =>
    del(`${API_ENDPOINTS.secretsKeys}?provider=${encodeURIComponent(provider)}`),

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
}
