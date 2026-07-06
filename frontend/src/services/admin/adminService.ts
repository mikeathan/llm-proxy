import { API_ENDPOINTS } from '../../constants/api'
import type { AdminState, APIKeyItem, GlobalConfig, ProcessListResponse, ProcessKillResponse, WebhookInfo } from '../../types/admin'
import type { Model, ProviderModelInfo } from '../../types/model'
import { get, post, put, del } from '../httpClient'

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

  fetchToolSecret: (category: string, provider: string): Promise<string> =>
    get<{ secret: string }>(`${API_ENDPOINTS.secretsTools}?category=${encodeURIComponent(category)}&provider=${encodeURIComponent(provider)}`)
      .then(r => r.secret),

  saveToolSecret: (category: string, provider: string, secret: string): Promise<void> =>
    put<void>(`${API_ENDPOINTS.secretsTools}?category=${encodeURIComponent(category)}&provider=${encodeURIComponent(provider)}`, { secret }),

  deleteToolSecret: (category: string, provider: string): Promise<void> =>
    del<void>(`${API_ENDPOINTS.secretsTools}?category=${encodeURIComponent(category)}&provider=${encodeURIComponent(provider)}`),

  createConnectorWebhook: (name: string, url: string): Promise<{ status: string; url: string }> =>
    post<{ status: string; url: string }>(API_ENDPOINTS.connectorWebhook(name), { action: 'create', url }),

  verifyConnectorWebhook: (name: string): Promise<WebhookInfo> =>
    post<WebhookInfo>(API_ENDPOINTS.connectorWebhook(name), { action: 'verify' }),

  deleteConnectorWebhook: (name: string): Promise<{ status: string }> =>
    post<{ status: string }>(API_ENDPOINTS.connectorWebhook(name), { action: 'delete' }),

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
