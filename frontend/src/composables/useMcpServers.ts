import { ref } from 'vue'
import { McpApiService } from '../services/mcpService'
import type { McpServer } from '../types/mcp'

const mcpServers = ref<McpServer[]>([])

const refresh = async (): Promise<void> => {
  try {
    mcpServers.value = await McpApiService.fetchAll()
  } catch (e: any) {
    console.error('[useMcpServers] fetch failed:', e.message)
  }
}

const addMCPServer = async (payload: Omit<McpServer, 'enabled'>): Promise<void> => {
  try {
    await McpApiService.add(payload)
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error adding MCP server: ${e.message}`)
  }
}

const toggleMCPServer = async (server: McpServer): Promise<void> => {
  try {
    await McpApiService.toggle(server)
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error toggling MCP server: ${e.message}`)
  }
}

const removeMCPServer = async (name: string): Promise<void> => {
  if (!confirm(`Remove MCP server "${name}"?`)) return
  try {
    await McpApiService.remove(name)
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error removing MCP server: ${e.message}`)
  }
}

export function useMcpServers() {
  return {
    mcpServers,
    refresh,
    addMCPServer,
    toggleMCPServer,
    removeMCPServer,
  }
}
