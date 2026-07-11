import { ref, computed, onMounted, onUnmounted } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { MetricsApiService } from '../../services/monitoring/metricsService'
import { McpApiService } from '../../services/mcp/mcpService'
import { useConfirm } from '../ui/useConfirm'
import type { AdminState, McpServer, ProcessLogs, SystemMetrics } from '../../types'

export function useAdminState() {
  const { confirm } = useConfirm()
  const state = ref<AdminState | null>(null)
  const metrics = ref<SystemMetrics | null>(null)
  const logs = ref<ProcessLogs | null>(null)
  const mcpServers = ref<McpServer[]>([])
  const logLevel = ref<string>('INFO')

  let pollInterval: any = null

  const activeModel = computed(() => state.value?.active)
  const availableModels = computed(() => state.value?.available || [])

  const fetchState = async () => {
    try {
      state.value = await AdminApiService.fetchState()
    } catch (e: any) {
      console.error('Failed to fetch state:', e.message)
    }
  }

  const fetchMetricsAndLogs = async () => {
    try {
      metrics.value = await MetricsApiService.fetchMetrics()
      logs.value = await MetricsApiService.fetchLogs()
    } catch (e: any) {
      console.error('Failed to fetch runtime metrics/logs:', e.message)
    }
  }

  const fetchMcpServers = async () => {
    try {
      mcpServers.value = await McpApiService.fetchAll()
    } catch (e: any) {
      console.error('Failed to fetch MCP servers:', e.message)
    }
  }

  const initialize = async () => {
    await fetchState()
    await fetchMetricsAndLogs()
    await fetchMcpServers()
    try {
      logLevel.value = await MetricsApiService.fetchLogLevel()
    } catch (e: any) {
      console.error('Failed to fetch log level:', e.message)
    }

    if (!pollInterval) {
      pollInterval = setInterval(() => {
        fetchState()
        fetchMetricsAndLogs()
      }, 5000)
    }
  }

  onMounted(() => {
    initialize()
  })

  onUnmounted(() => {
    if (pollInterval) {
      clearInterval(pollInterval)
      pollInterval = null
    }
  })

  const startModel = async (name: string) => {
    try {
      await AdminApiService.startModel(name)
      await fetchState()
    } catch (e: any) {
      console.error(e)
      alert(`Error starting model: ${e.message}`)
    }
  }

  const stopModel = async () => {
    try {
      await AdminApiService.stopModel()
      await fetchState()
    } catch (e: any) {
      console.error(e)
      alert(`Error stopping model: ${e.message}`)
    }
  }

  const addModel = async (payload: any) => {
    try {
      await AdminApiService.addModel(payload)
      await fetchState()
    } catch (e: any) {
      console.error(e)
      alert(`Error adding model: ${e.message}`)
    }
  }

  const updateModel = async (payload: any) => {
    try {
      await AdminApiService.updateModel(payload)
      await fetchState()
    } catch (e: any) {
      console.error(e)
      alert(`Error updating model: ${e.message}`)
    }
  }

  const removeModel = async (name: string) => {
    const confirmed = await confirm({
      title: 'Remove Model',
      message: `Are you sure you want to remove model "${name}"?`,
      type: 'error',
      confirmText: 'Remove',
      cancelText: 'Cancel'
    })
    if (!confirmed) return
    try {
      await AdminApiService.removeModel(name)
      await fetchState()
    } catch (e: any) {
      console.error(e)
      alert(`Error removing model: ${e.message}`)
    }
  }

  const updateConfig = async (payload: any) => {
    try {
      await AdminApiService.updateConfig(payload)
      await fetchState()
      alert('Configuration saved')
    } catch (e: any) {
      console.error(e)
      alert(`Error updating config: ${e.message}`)
    }
  }

  const updateLogLevel = async (level: string) => {
    try {
      await MetricsApiService.updateLogLevel(level as any)
      logLevel.value = level
    } catch (e: any) {
      console.error(e)
      alert(`Error updating log level: ${e.message}`)
    }
  }

  const addMCPServer = async (payload: any) => {
    try {
      await McpApiService.add(payload)
      await fetchMcpServers()
    } catch (e: any) {
      console.error(e)
      alert(`Error adding MCP server: ${e.message}`)
    }
  }

  const removeMCPServer = async (name: string) => {
    const confirmed = await confirm({
      title: 'Remove MCP Server',
      message: `Are you sure you want to remove the MCP server "${name}"?`,
      type: 'error',
      confirmText: 'Remove',
      cancelText: 'Cancel'
    })
    if (!confirmed) return
    try {
      await McpApiService.remove(name)
      await fetchMcpServers()
    } catch (e: any) {
      console.error(e)
      alert(`Error removing MCP server: ${e.message}`)
    }
  }

  const toggleMCPServer = async (server: any) => {
    try {
      await McpApiService.toggle(server)
      await fetchMcpServers()
    } catch (e: any) {
      console.error(e)
      alert(`Error toggling MCP server: ${e.message}`)
    }
  }

  return {
    state,
    metrics,
    logs,
    mcpServers,
    logLevel,
    activeModel,
    availableModels,
    startModel,
    stopModel,
    addModel,
    updateModel,
    removeModel,
    updateConfig,
    updateLogLevel,
    addMCPServer,
    removeMCPServer,
    toggleMCPServer
  }
}
