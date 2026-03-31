<script setup lang="ts">
import { ref, watch } from 'vue'
import GlobalSettings from '../components/GlobalSettings.vue'
import McpServers from '../components/McpServers.vue'
import { useConfig } from '../composables/useConfig'
import { useMcpServers } from '../composables/useMcpServers'
import { useMetrics } from '../composables/useMetrics'
import { useModels } from '../composables/useModels'
import type { NewMcpServerForm } from '../types/mcp'

const { refresh: refreshModels, state } = useModels()
const { editConfig, seedFromState, updateConfig } = useConfig(refreshModels)
const { mcpServers, addMCPServer, toggleMCPServer, removeMCPServer } = useMcpServers()
const { logLevel, updateLogLevel } = useMetrics()

// Seed editConfig the first time state.config arrives
watch(() => state.value?.config, (cfg) => {
  if (cfg) seedFromState(cfg)
}, { immediate: true })

const newMcpServer = ref<NewMcpServerForm>({ name: '', url: '' })

const handleAddMCPServer = (): void => {
  if (!newMcpServer.value.name || !newMcpServer.value.url) return
  addMCPServer(newMcpServer.value)
  newMcpServer.value = { name: '', url: '' }
}
</script>

<template>
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <GlobalSettings
      v-model:editConfig="editConfig"
      :logLevel="logLevel"
      @updateConfig="updateConfig"
      @updateLogLevel="updateLogLevel"
    />
    <McpServers
      :mcpServers="mcpServers"
      v-model:newMcpServer="newMcpServer"
      @addMCPServer="handleAddMCPServer"
      @toggleMCPServer="toggleMCPServer"
      @removeMCPServer="removeMCPServer"
    />
  </div>
</template>
