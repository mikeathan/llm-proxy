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

async function del(url: string): Promise<void> {
  const res = await fetch(url, { method: 'DELETE' })
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as Record<string, string>
    throw new Error(err['error'] || res.statusText)
  }
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

  updateConfig: (payload: Partial<GlobalConfig>): Promise<void> =>
    put<void>(API_ENDPOINTS.config, payload),
  
  fetchProviderModels: (provider: string): Promise<string[]> =>
    get<string[]>(`${API_ENDPOINTS.providerModels}?provider=${encodeURIComponent(provider)}`),

  testConnection: (provider: string): Promise<{ status: string; message: string }> =>
    get<{ status: string; message: string }>(`${API_ENDPOINTS.testConnection}?provider=${encodeURIComponent(provider)}`),
}
