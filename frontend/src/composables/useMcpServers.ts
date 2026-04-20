import { ref } from 'vue'
import { McpApiService } from '../services/mcpService'
import { useToast } from './useToast'
import type { McpServer } from '../types/mcp'

const { error: toastError, success: toastSuccess } = useToast()

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
		await McpApiService.add({ ...payload, enabled: false } as McpServer)
		toastSuccess(`MCP Server "${payload.name}" added`)
		await refresh()
	} catch (e: any) {
		console.error(e)
		toastError(`Error adding MCP server: ${e.message}`)
	}
}

const toggleMCPServer = async (server: McpServer): Promise<void> => {
	try {
		await McpApiService.toggle(server)
		toastSuccess(`${server.name} ${!server.enabled ? 'enabled' : 'disabled'}`)
		await refresh()
	} catch (e: any) {
		console.error(e)
		toastError(`Error toggling MCP server: ${e.message}`)
	}
}

const removeMCPServer = async (name: string): Promise<void> => {
	if (!confirm(`Remove MCP server "${name}"?`)) return
	try {
		await McpApiService.remove(name)
		toastSuccess(`MCP Server "${name}" removed`)
		await refresh()
	} catch (e: any) {
		console.error(e)
		toastError(`Error removing MCP server: ${e.message}`)
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
