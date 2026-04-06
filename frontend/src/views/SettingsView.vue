<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import GlobalSettings from '../components/settings/GlobalSettings.vue'
import McpServers from '../components/settings/McpServers.vue'
import ApiKeySettings from '../components/settings/ApiKeySettings.vue'
import { useConfig } from '../composables/useConfig'
import { useMcpServers } from '../composables/useMcpServers'
import { useMetrics } from '../composables/useMetrics'
import { useModels } from '../composables/useModels'
import { AdminApiService } from '../services/adminService'
import type { NewMcpServerForm } from '../types/mcp'
import type { ProviderType, APIKeyItem } from '../types/admin'

const { config, updateConfig, fetchConfig, isSaving, isLoading, ensureProvider } = useConfig()
const { refresh: refreshModels } = useModels()
const { mcpServers, addMCPServer, toggleMCPServer, removeMCPServer } = useMcpServers()
const { logLevel, updateLogLevel } = useMetrics()

type Tab = 'local' | 'gemini' | 'openai' | 'openrouter' | 'mcp'
const activeTab = ref<Tab>('local')
const testStatus = ref<{ [key: string]: { loading: boolean; error?: string; success?: string } }>({})

// Pre-process key lists as computed to avoid creating new arrays on every render
const geminiKeys = computed<APIKeyItem[]>(() => ensureIds(config.value.providers?.gemini?.api_keys ?? []))
const openaiKeys = computed<APIKeyItem[]>(() => ensureIds(config.value.providers?.openai?.api_keys ?? []))
const openrouterKeys = computed<APIKeyItem[]>(() => ensureIds(config.value.providers?.openrouter?.api_keys ?? []))

function setTab(tab: Tab) {
  activeTab.value = tab
}

function ensureIds(keys: any[]): APIKeyItem[] {
  return keys.map(k => {
    if (k.id) return k
    return {
      ...k,
      id: typeof crypto !== 'undefined' && crypto.randomUUID
        ? crypto.randomUUID()
        : Math.random().toString(36).substring(2, 11)
    }
  })
}

async function updateApiKeys(type: ProviderType, keys: APIKeyItem[]) {
  const provider = ensureProvider(type)
  provider.api_keys = ensureIds(keys)
  try {
    await AdminApiService.updateConfig(JSON.parse(JSON.stringify(config.value)))
    await refreshModels()
  } catch (e: any) {
    console.error(`[Settings] Failed to auto-save ${type} API keys:`, e)
  }
}

async function handleSaveConfig() {
  try {
    await updateConfig()
    alert('Configuration saved successfully')
    await refreshModels()
  } catch (e: any) {
    alert(`Error saving configuration: ${e.message}`)
  }
}

const testProvider = async (type: string, apiKey?: string) => {
  testStatus.value[type] = { loading: true }
  try {
    const res = await AdminApiService.testConnection(type, apiKey)
    testStatus.value[type] = { loading: false, success: res.message }
    setTimeout(() => {
      if (testStatus.value[type]?.success === res.message) {
        testStatus.value[type] = { loading: false }
      }
    }, 5000)
  } catch (e: any) {
    testStatus.value[type] = { loading: false, error: e.message }
  }
}

function clearTestStatus(type: string) {
  testStatus.value[type] = { loading: false }
}

const newMcpServer = ref<NewMcpServerForm>({ name: '', url: '' })
const handleAddMCPServer = (): void => {
  if (!newMcpServer.value.name || !newMcpServer.value.url) return
  addMCPServer(newMcpServer.value)
  newMcpServer.value = { name: '', url: '' }
}

onMounted(() => {
  fetchConfig()
})
</script>

<template>
  <div class="settings-layout">
    <!-- Sidebar -->
    <div class="settings-sidebar">
      <h2 class="sidebar-title">Settings</h2>
      <nav class="sidebar-nav">
        <button @click="setTab('local')"      :class="['nav-item', activeTab === 'local'      ? 'nav-active' : '']"><span class="nav-icon">💻</span> Local Engine</button>
        <button @click="setTab('gemini')"     :class="['nav-item', activeTab === 'gemini'     ? 'nav-active' : '']"><span class="nav-icon">✨</span> Gemini</button>
        <button @click="setTab('openai')"     :class="['nav-item', activeTab === 'openai'     ? 'nav-active' : '']"><span class="nav-icon">🤖</span> OpenAI</button>
        <button @click="setTab('openrouter')" :class="['nav-item', activeTab === 'openrouter' ? 'nav-active' : '']"><span class="nav-icon">🚀</span> OpenRouter</button>
        <div class="sidebar-divider"></div>
        <button @click="setTab('mcp')"        :class="['nav-item', activeTab === 'mcp'        ? 'nav-active' : '']"><span class="nav-icon">🔌</span> MCP Servers</button>
      </nav>
    </div>

    <!-- Main Content -->
    <div class="settings-detail">
      <!-- Loading -->
      <div v-if="isLoading" class="detail-card flex justify-center py-20">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>

      <template v-else>
        <!-- Local Engine -->
        <div v-show="activeTab === 'local'">
          <GlobalSettings
            v-model:editConfig="config"
            :logLevel="logLevel"
            @updateConfig="handleSaveConfig"
            @updateLogLevel="updateLogLevel"
          />
        </div>

        <!-- Google Gemini -->
        <div v-show="activeTab === 'gemini'">
          <div class="detail-card">
            <h2 class="detail-title">Google Gemini Configuration</h2>
            <form @submit.prevent="handleSaveConfig" class="space-y-6">
              <ApiKeySettings
                :apiKeys="geminiKeys"
                title="API Keys"
                helperText="Select a key to test or edit it. Changes are saved automatically."
                :testLoading="!!testStatus['gemini']?.loading"
                :testSuccess="testStatus['gemini']?.success"
                :testError="testStatus['gemini']?.error"
                @update:apiKeys="updateApiKeys('gemini', $event)"
                @testKey="testProvider('gemini', $event)"
                @clearTest="clearTestStatus('gemini')"
              />
              <div class="sidebar-divider op-10"></div>
              <div v-if="config.providers?.gemini" class="form-group">
                <label class="form-label">Project ID <span class="form-optional">(Optional)</span></label>
                <div class="form-helper">Required for Vertex AI</div>
                <input v-model="config.providers.gemini.project_id" type="text" class="form-input">
              </div>
              <div v-if="config.providers?.gemini" class="form-group">
                <label class="form-label">Region <span class="form-optional">(Optional)</span></label>
                <div class="form-helper">Region for Vertex AI (e.g. us-central1)</div>
                <input v-model="config.providers.gemini.region" type="text" class="form-input">
              </div>
              <div class="form-actions">
                <button type="submit" class="btn-submit" :disabled="isSaving">
                  {{ isSaving ? 'Saving...' : 'Save Gemini Config' }}
                </button>
              </div>
            </form>
          </div>
        </div>

        <!-- OpenAI -->
        <div v-show="activeTab === 'openai'">
          <div class="detail-card">
            <h2 class="detail-title">OpenAI Configuration</h2>
            <form @submit.prevent="handleSaveConfig" class="space-y-6">
              <ApiKeySettings
                :apiKeys="openaiKeys"
                title="API Keys"
                helperText="Select a key to test or edit it. Changes are saved automatically."
                :testLoading="!!testStatus['openai']?.loading"
                :testSuccess="testStatus['openai']?.success"
                :testError="testStatus['openai']?.error"
                @update:apiKeys="updateApiKeys('openai', $event)"
                @testKey="testProvider('openai', $event)"
                @clearTest="clearTestStatus('openai')"
              />
              <div class="sidebar-divider op-10"></div>
              <div v-if="config.providers?.openai" class="form-group">
                <label class="form-label">Base URL <span class="form-optional">(Optional)</span></label>
                <div class="form-helper">Override for localized proxies or self-hosted engines</div>
                <input v-model="config.providers.openai.base_url" type="text" placeholder="https://api.openai.com/v1" class="form-input">
              </div>
              <div class="form-actions">
                <button type="submit" class="btn-submit" :disabled="isSaving">
                  {{ isSaving ? 'Saving...' : 'Save OpenAI Config' }}
                </button>
              </div>
            </form>
          </div>
        </div>

        <!-- OpenRouter -->
        <div v-show="activeTab === 'openrouter'">
          <div class="detail-card">
            <h2 class="detail-title">OpenRouter Configuration</h2>
            <form @submit.prevent="handleSaveConfig" class="space-y-6">
              <ApiKeySettings
                :apiKeys="openrouterKeys"
                title="API Keys"
                helperText="Select a key to test or edit it. Changes are saved automatically."
                :testLoading="!!testStatus['openrouter']?.loading"
                :testSuccess="testStatus['openrouter']?.success"
                :testError="testStatus['openrouter']?.error"
                @update:apiKeys="updateApiKeys('openrouter', $event)"
                @testKey="testProvider('openrouter', $event)"
                @clearTest="clearTestStatus('openrouter')"
              />
              <div class="form-actions">
                <button type="submit" class="btn-submit" :disabled="isSaving">
                  {{ isSaving ? 'Saving...' : 'Save OpenRouter Config' }}
                </button>
              </div>
            </form>
          </div>
        </div>

        <!-- MCP Servers -->
        <div v-show="activeTab === 'mcp'">
          <McpServers
            :mcpServers="mcpServers"
            v-model:newMcpServer="newMcpServer"
            @addMCPServer="handleAddMCPServer"
            @toggleMCPServer="toggleMCPServer"
            @removeMCPServer="removeMCPServer"
          />
        </div>
      </template>
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
.form-optional {
  @apply text-xs font-normal text-gray-500;
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
  @apply bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white px-6 py-2.5 rounded-md font-bold transition-all shadow-lg hover:shadow-blue-600/20 active:scale-95;
}
.op-10 {
  @apply opacity-10;
}
</style>
