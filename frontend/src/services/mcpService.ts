import { API_ENDPOINTS } from '../constants/api'
import type { McpServer } from '../types/mcp'

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as Record<string, string>
    throw new Error(err['error'] || res.statusText)
  }
  return res.json() as Promise<T>
}

export const McpApiService = {
  fetchAll: async (): Promise<McpServer[]> => {
    const res = await fetch(API_ENDPOINTS.mcp)
    const data = await handleResponse<McpServer[] | null>(res)
    return Array.isArray(data) ? data : []
  },

  add: (payload: Omit<McpServer, 'enabled'>): Promise<McpServer> => {
    return fetch(API_ENDPOINTS.mcp, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }).then(r => handleResponse<McpServer>(r))
  },

  update: (payload: McpServer): Promise<McpServer> => {
    return fetch(API_ENDPOINTS.mcp, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }).then(r => handleResponse<McpServer>(r))
  },

  toggle: (server: McpServer): Promise<McpServer> => {
    return McpApiService.update({ ...server, enabled: !server.enabled })
  },

  remove: async (name: string): Promise<void> => {
    const res = await fetch(`${API_ENDPOINTS.mcp}?name=${encodeURIComponent(name)}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({})) as Record<string, string>
      throw new Error(err['error'] || res.statusText)
    }
  },
}
