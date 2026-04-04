<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import GlobalSettings from '../components/GlobalSettings.vue'
import McpServers from '../components/McpServers.vue'
import { useConfig } from '../composables/useConfig'
import { useMcpServers } from '../composables/useMcpServers'
import { useMetrics } from '../composables/useMetrics'
import { useModels } from '../composables/useModels'
import type { NewMcpServerForm } from '../types/mcp'
import type { ProviderType } from '../types/admin'

const { refresh: refreshModels, state } = useModels()
const { editConfig, seedFromState, updateConfig } = useConfig(refreshModels)
const { mcpServers, addMCPServer, toggleMCPServer, removeMCPServer } = useMcpServers()
const { logLevel, updateLogLevel } = useMetrics()

const activeTab = ref<'local' | 'gemini' | 'openai' | 'mcp'>('local')

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

const ensureProvider = (type: ProviderType) => {
  if (!editConfig.value.providers[type]) {
    editConfig.value.providers[type] = { type }
  }
  return editConfig.value.providers[type]
}

const geminiProvider = computed(() => ensureProvider('gemini'))
const openaiProvider = computed(() => ensureProvider('openai'))
</script>

<template>
  <div class="settings-layout">
    <!-- Sidebar -->
    <div class="settings-sidebar">
      <h2 class="sidebar-title">Settings</h2>
      <nav class="sidebar-nav">
        <button
          @click="activeTab = 'local'"
          :class="['nav-item', activeTab === 'local' ? 'nav-active' : '']"
        >
          <span class="nav-icon">💻</span>
          Local Engine
        </button>
        <button
          @click="activeTab = 'gemini'"
          :class="['nav-item', activeTab === 'gemini' ? 'nav-active' : '']"
        >
          <span class="nav-icon">✨</span>
          Google Gemini
        </button>
        <button
          @click="activeTab = 'openai'"
          :class="['nav-item', activeTab === 'openai' ? 'nav-active' : '']"
        >
          <span class="nav-icon">🤖</span>
          OpenAI
        </button>
        <div class="sidebar-divider"></div>
        <button
          @click="activeTab = 'mcp'"
          :class="['nav-item', activeTab === 'mcp' ? 'nav-active' : '']"
        >
          <span class="nav-icon">🔌</span>
          MCP Servers
        </button>
      </nav>
    </div>

    <!-- Main Content -->
    <div class="settings-detail">
      <div v-show="activeTab === 'local'">
        <GlobalSettings
          v-model:editConfig="editConfig"
          :logLevel="logLevel"
          @updateConfig="updateConfig"
          @updateLogLevel="updateLogLevel"
        />
      </div>

      <div v-show="activeTab === 'gemini'">
        <div class="detail-card">
          <h2 class="detail-title">Google Gemini Configuration</h2>
          <form @submit.prevent="updateConfig" class="space-y-6">
            <div class="form-group">
              <label class="form-label">API Key</label>
              <div class="form-helper">Google AI Studio or Vertex AI key</div>
              <input v-model="geminiProvider.api_key" type="password" placeholder="AIza..." class="form-input">
            </div>
            <div class="form-group">
              <label class="form-label">Project ID (Optional)</label>
              <div class="form-helper">Required for Vertex AI</div>
              <input v-model="geminiProvider.project_id" type="text" class="form-input">
            </div>
            <div class="form-group">
              <label class="form-label">Region (Optional)</label>
              <div class="form-helper">Region for Vertex AI (e.g. us-central1)</div>
              <input v-model="geminiProvider.region" type="text" class="form-input">
            </div>
            <div class="form-actions">
              <button type="submit" class="btn-submit">Save Gemini Config</button>
            </div>
          </form>
        </div>
      </div>

      <div v-show="activeTab === 'openai'">
        <div class="detail-card">
          <h2 class="detail-title">OpenAI Configuration</h2>
          <form @submit.prevent="updateConfig" class="space-y-6">
            <div class="form-group">
              <label class="form-label">API Key</label>
              <input v-model="openaiProvider.api_key" type="password" placeholder="sk-..." class="form-input">
            </div>
            <div class="form-group">
              <label class="form-label">Base URL (Optional)</label>
              <div class="form-helper">Override for localized proxies or self-hosted engines</div>
              <input v-model="openaiProvider.base_url" type="text" placeholder="https://api.openai.com/v1" class="form-input">
            </div>
            <div class="form-actions">
              <button type="submit" class="btn-submit">Save OpenAI Config</button>
            </div>
          </form>
        </div>
      </div>

      <div v-show="activeTab === 'mcp'">
        <McpServers
          :mcpServers="mcpServers"
          v-model:newMcpServer="newMcpServer"
          @addMCPServer="handleAddMCPServer"
          @toggleMCPServer="toggleMCPServer"
          @removeMCPServer="removeMCPServer"
        />
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.settings-layout {
  @apply flex flex-col lg:flex-row gap-6 h-full;
}

.settings-sidebar {
  @apply w-full lg:w-64 bg-gray-800 rounded-lg border border-gray-700 p-4 shrink-0 h-fit;
}

.sidebar-title {
  @apply text-sm font-bold text-gray-500 uppercase tracking-wider mb-4 px-3;
}

.sidebar-nav {
  @apply space-y-1;
}

.nav-item {
  @apply w-full text-left px-3 py-2.5 rounded-md text-sm font-medium transition-all flex items-center gap-3 text-gray-400 hover:text-white hover:bg-gray-700;
}

.nav-active {
  @apply bg-blue-600/10 text-blue-400 border border-blue-600/30;
}

.nav-icon {
  @apply text-base grayscale brightness-50;
}

.nav-active .nav-icon {
  @apply grayscale-0 brightness-100;
}

.sidebar-divider {
  @apply h-px bg-gray-700 my-4 mx-2;
}

.settings-detail {
  @apply flex-1 min-w-0;
}

/* Common form styles for local tabs */
.detail-card {
  @apply bg-gray-800 rounded-lg border border-gray-700 p-6 shadow-xl animate-in fade-in duration-300;
}

.detail-title {
  @apply text-xl font-bold text-white mb-6 border-b border-gray-700 pb-3;
}

.form-group {
  @apply space-y-1.5;
}

.form-label {
  @apply block text-sm font-semibold text-gray-200;
}

.form-helper {
  @apply text-xs text-gray-500 mb-2;
}

.form-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded-md px-3 py-2.5 text-white transition-all focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}

.form-actions {
  @apply pt-6 border-t border-gray-700 flex justify-end;
}

.btn-submit {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-6 py-2.5 rounded-md font-bold transition-all shadow-lg hover:shadow-blue-600/20 active:scale-95;
}
</style>
