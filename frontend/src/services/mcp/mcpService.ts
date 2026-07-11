import { API_ENDPOINTS } from '../../constants/api'
import type { McpServer } from '../../types/mcp'
import { get, post, put, del } from '../httpClient'

export const McpApiService = {
  fetchAll: async (): Promise<McpServer[]> => {
    const data = await get<McpServer[] | null>(API_ENDPOINTS.mcp)
    return Array.isArray(data) ? data : []
  },

  add: (payload: Omit<McpServer, 'enabled'>): Promise<McpServer> =>
    post<McpServer>(API_ENDPOINTS.mcp, payload),

  update: (payload: McpServer): Promise<McpServer> =>
    put<McpServer>(API_ENDPOINTS.mcp, payload),

  toggle: (server: McpServer): Promise<McpServer> =>
    put<McpServer>(API_ENDPOINTS.mcp, { ...server, enabled: !server.enabled }),

  remove: (name: string): Promise<void> =>
    del<void>(`${API_ENDPOINTS.mcp}?name=${encodeURIComponent(name)}`),
}
