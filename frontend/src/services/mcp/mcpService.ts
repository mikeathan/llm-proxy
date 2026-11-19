import { API_ENDPOINTS } from '../../constants/api'
import type { McpServer, McpServerBody, McpServersAnswer, McpTestRequest } from '../../types/mcp'
import { del, get, post, put } from '../httpClient'

// The writes need the passphrase again; the shared HTTP client asks for it and sends the request once more.
export const McpApiService = {
  list: (): Promise<McpServersAnswer> => get<McpServersAnswer>(API_ENDPOINTS.mcpServers),
  add: (body: McpServerBody): Promise<McpServer> => post<McpServer>(API_ENDPOINTS.mcpServers, body),
  update: (body: McpServerBody): Promise<McpServer> => put<McpServer>(API_ENDPOINTS.mcpServers, body),
  remove: (name: string): Promise<void> => del<void>(`${API_ENDPOINTS.mcpServers}?name=${encodeURIComponent(name)}`),
  test: (request: McpTestRequest): Promise<McpServer> => post<McpServer>(API_ENDPOINTS.mcpTest, request),
}
