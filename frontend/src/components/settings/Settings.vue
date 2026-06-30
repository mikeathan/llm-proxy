<script setup lang="ts">
import { ref, onMounted, computed, inject, type Ref } from "vue";
import GlobalSettings from "./GlobalSettings.vue";
import SecuritySettings from "./SecuritySettings.vue";
import McpServers from "./McpServers.vue";
import ApiKeySettings from "./ApiKeySettings.vue";
import GuardrailSettings from "./GuardrailSettings.vue";
import ProviderModelsCard from "./ProviderModelsCard.vue";
import InfrastructurePanel from "../infrastructure/InfrastructurePanel.vue";
import CommunicationSettings from "./CommunicationSettings.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import { useConfig } from "../../composables/models/useConfig";
import { useMcpServers } from "../../composables/system/useMcpServers";
import { useMetrics } from "../../composables/system/useMetrics";
import { useModels } from "../../composables/models/useModels";
import { AdminApiService } from "../../services/adminService";
import { useProviders } from "../../composables/models/useProviders";
import { useToast } from "../../composables/useToast";
import type { NewMcpServerForm } from "../../types/mcp";
import type { ProviderType, APIKeyItem, SettingsTab } from "../../types/admin";
import { isProviderTab, getSettingsGroups } from "../../domain/settings";

const {
  config,
  updateConfig,
  fetchConfig,
  isSaving,
  isLoading,
  ensureProvider,
} = useConfig();
const { state: adminModelsState, availableModels, refresh: refreshModels } = useModels();
const modelsList = computed(() => adminModelsState.value?.models || []);
const modelsByProvider = computed(() => {
  const all = adminModelsState.value?.models || [];
  const grouped: Record<string, typeof all> = {};
  for (const m of all) {
    if (!grouped[m.provider]) grouped[m.provider] = [];
    grouped[m.provider]!.push(m);
  }
  return grouped;
});

function modelCountsByKey(provider: string): Record<string, number> {
  const models = modelsByProvider.value[provider] || [];
  const counts: Record<string, number> = {};
  for (const m of models) {
    const key = m.provider_config?.api_key_name || "";
    counts[key] = (counts[key] || 0) + 1;
  }
  return counts;
}
const { mcpServers, addMCPServer, toggleMCPServer, removeMCPServer } =
  useMcpServers();
const { settingsTabs, getIcon, getLabel, fetchManifests } =
  useProviders();
const { logLevel, updateLogLevel } = useMetrics();
const toast = useToast();

const activeTab = inject<Ref<SettingsTab>>("activeSettingsTab", ref("local"));
const testStatus = ref<{
  [key: string]: { loading: boolean; error?: string; success?: string };
}>({});

// Dynamic key list — loaded from secrets API per-provider on demand
const providerKeys = ref<Record<string, APIKeyItem[]>>({});

async function fetchProviderKeysForTab(provider: ProviderType) {
  try {
    const keys = await AdminApiService.fetchProviderKeys(provider);
    providerKeys.value = { ...providerKeys.value, [provider]: keys };
  } catch (e) {
    console.error(`[Settings] Failed to load keys for ${provider}:`, e);
  }
}

function setTab(tab: SettingsTab) {
  activeTab.value = tab;
  if (isProviderTab(tab)) {
    ensureProvider(tab);
    fetchProviderKeysForTab(tab as ProviderType);
  }
}

async function updateApiKeys(type: ProviderType, keys: APIKeyItem[]) {
  try {
    const saved = await AdminApiService.saveProviderKeys(type, keys);
    // Update local state with what the server returned (masked)
    providerKeys.value = { ...providerKeys.value, [type]: saved };
    await refreshModels();
  } catch (e: any) {
    toast.error(`Failed to save API keys: ${e.message}`);
    console.error(`[Settings] Failed to save ${type} API keys:`, e);
  }
}

async function clearAllApiKeys(type: ProviderType) {
  try {
    const saved = await AdminApiService.deleteAllProviderKeys(type);
    providerKeys.value = { ...providerKeys.value, [type]: saved };
    await refreshModels();
  } catch (e: any) {
    toast.error(`Failed to clear API keys: ${e.message}`);
    console.error(`[Settings] Failed to clear ${type} API keys:`, e);
  }
}

async function handleSaveConfig() {
  try {
    await updateConfig();
    toast.success("Configuration saved successfully");
    await refreshModels();
  } catch (e: any) {
    toast.error(`Error saving configuration: ${e.message}`);
  }
}

const testProvider = async (type: string, payload: { key: string; name: string; id: string; base_url?: string }) => {
  testStatus.value[type] = { loading: true };
  try {
    const baseURL = payload.base_url || (config.value.providers as any)?.[type]?.base_url;
    const res = await AdminApiService.testConnection(type, payload.key, payload.id, baseURL);
    testStatus.value[type] = { loading: false, success: res.message };
    setTimeout(() => {
      if (testStatus.value[type]?.success === res.message) {
        testStatus.value[type] = { loading: false };
      }
    }, 5000);
  } catch (e: any) {
    testStatus.value[type] = { loading: false, error: e.message };
  }
};

// OpenAI-compatible providers that support per-key base URLs
const openAICompatibleProviders = new Set(['openai', 'openrouter', 'mulerouter', 'nvidia']);
const providerHasBaseUrl = (p: string) => openAICompatibleProviders.has(p);

function clearTestStatus(type: string) {
  testStatus.value[type] = { loading: false };
}

const newMcpServer = ref<NewMcpServerForm>({ name: "", url: "" });
const handleAddMCPServer = (): void => {
  if (!newMcpServer.value.name || !newMcpServer.value.url) return;
  addMCPServer(newMcpServer.value);
  newMcpServer.value = { name: "", url: "" };
};

async function handleRestartBackend() {
  try {
    toast.success("Restart request sent. Reconnecting in 5 seconds...")
    await AdminApiService.restartSystem()
    // The backend will exit. We wait a bit then refresh.
    setTimeout(() => {
      window.location.reload()
    }, 5000)
  } catch (e: any) {
    toast.error(`Failed to request restart: ${e.message}`)
  }
}

onMounted(() => {
  fetchManifests();
  fetchConfig();
  refreshModels();
});

const settingsGroups = computed(() => getSettingsGroups(settingsTabs.value));
</script>

<template>
  <div class="settings-shell">
    <!-- Sidebar -->
    <div class="settings-sidebar">
      <h2 class="sidebar-header">Preferences</h2>
      <div v-for="group in settingsGroups" :key="group.name" class="nav-group">
        <h3 v-if="group.tabs.length > 0" class="group-title">
          {{ group.name }}
        </h3>
        <nav class="nav-list">
          <button
            v-for="tab in group.tabs"
            :key="tab"
            @click="setTab(tab)"
            class="nav-item"
            :class="
              activeTab === tab ? 'nav-item--active' : 'nav-item--inactive'
            "
          >
            <span
              class="nav-icon"
              :class="
                activeTab === tab ? 'nav-icon--active' : 'nav-icon--inactive'
              "
            >
              {{ getIcon(tab) }}
            </span>
            <span class="tab-label">{{ getLabel(tab) }}</span>
          </button>
        </nav>
      </div>
    </div>

    <!-- Main Content -->
    <div class="settings-content">
      <!-- Loading -->
      <div v-if="isLoading" class="loading-card">
        <div class="spinner"></div>
      </div>

      <template v-else>
        <!-- Local Engine -->
        <div v-show="activeTab === 'local'">
          <GlobalSettings
            v-model:editConfig="config"
            :logLevel="logLevel"
            :models="modelsList"
            @updateConfig="handleSaveConfig"
            @updateLogLevel="updateLogLevel"
            @restartBackend="handleRestartBackend"
          />
        </div>

        <!-- Local Models -->
        <div v-show="activeTab === 'local-models'">
          <ProviderModelsCard
            :provider="'local'"
            :apiKeys="[]"
            :models="modelsByProvider['local'] || []"
            :availableModels="availableModels"
            @refresh="refreshModels"
          />
        </div>

        <!-- Local Host Terminal -->
        <div v-show="activeTab === 'security'">
          <SecuritySettings />
        </div>

        <!-- Guardrails -->
        <div v-show="activeTab === 'guardrails'">
          <GuardrailSettings v-model:config="config" @save="handleSaveConfig" />
        </div>

        <!-- Model Processes -->
        <div v-show="activeTab === 'processes'">
          <div class="config-card">
            <InfrastructurePanel />
          </div>
        </div>

        <!-- Communication -->
        <div v-show="activeTab === 'communication'">
          <CommunicationSettings
            :editConfig="config"
            @update:editConfig="(val: any) => config = val"
            @updateConfig="handleSaveConfig"
          />
        </div>

        <!-- Provider Configs -->
        <div
          v-for="provider in settingsTabs.filter(
            isProviderTab,
          ) as ProviderType[]"
          :key="provider"
          v-show="activeTab === provider"
        >
          <div class="config-card">
            <h2 class="config-header">
              {{ getLabel(provider) }} Configuration
            </h2>
            <form @submit.prevent="handleSaveConfig" class="form-section">
              <ApiKeySettings
                :apiKeys="providerKeys[provider] || []"
                title="API Keys"
                helperText="Select a key to test or edit it. Changes are saved automatically."
                :showBaseUrl="providerHasBaseUrl(provider)"
                :testLoading="!!testStatus[provider]?.loading"
                :testSuccess="testStatus[provider]?.success"
                :testError="testStatus[provider]?.error"
                :modelCounts="modelCountsByKey(provider)"
                @update:apiKeys="updateApiKeys(provider, $event)"
                @testKey="testProvider(provider, $event)"
                @clearTest="clearTestStatus(provider)"
                @clearAll="clearAllApiKeys(provider)"
              />

              <div class="form-divider"></div>

              <ProviderModelsCard
                :provider="provider"
                :apiKeys="providerKeys[provider] || []"
                :models="modelsByProvider[provider] || []"
                @refresh="refreshModels"
              />

              <div class="form-divider"></div>

              <template
                v-if="provider === 'gemini' && config.providers?.gemini"
              >
                <div class="form-group">
                  <label class="form-label"
                    >Project ID
                    <span class="form-optional">(Optional)</span></label
                  >
                  <div class="form-helper">Required for Vertex AI</div>
                  <input
                    v-model="config.providers.gemini.project_id"
                    type="text"
                    class="form-input"
                  />
                </div>
                <div class="form-group">
                  <label class="form-label"
                    >Region <span class="form-optional">(Optional)</span></label
                  >
                  <div class="form-helper">
                    Region for Vertex AI (e.g. us-central1)
                  </div>
                  <input
                    v-model="config.providers.gemini.region"
                    type="text"
                    class="form-input"
                  />
                </div>
              </template>

              <template
                v-if="provider === 'vertex' && config.providers?.vertex"
              >
                <div class="form-group">
                  <label class="form-label">Project ID</label>
                  <div class="form-helper">GCP Project ID</div>
                  <input
                    v-model="config.providers.vertex.project_id"
                    type="text"
                    class="form-input"
                    required
                  />
                </div>
                <div class="form-group">
                  <label class="form-label">Region</label>
                  <div class="form-helper">GCP Region (e.g. us-central1)</div>
                  <input
                    v-model="config.providers.vertex.region"
                    type="text"
                    class="form-input"
                    required
                  />
                </div>
              </template>

              <div class="form-actions">
                <BaseButton 
                  type="submit" 
                  variant="primary" 
                  :loading="isSaving" 
                  icon="play"
                  className="w-full"
                >
                  Save {{ provider }} Configuration
                </BaseButton>
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
.settings-shell {
  @apply flex flex-col lg:flex-row gap-6 h-full;
}

.settings-sidebar {
  @apply w-full lg:w-64 bg-gray-800 rounded-lg border border-gray-700 p-4 shrink-0 h-fit;
}

.sidebar-header {
  @apply text-lg font-black text-white px-3 mb-6 tracking-tight;
}

.nav-group {
  @apply mb-6;
}

.group-title {
  @apply text-[10px] font-bold text-gray-500 uppercase tracking-[0.2em] mb-3 px-3;
}

.nav-list {
  @apply space-y-0.5;
}

.nav-item {
  @apply w-full text-left px-3 py-2.5 rounded-md text-sm font-medium transition-all flex items-center gap-3 border;
}

.nav-item--active {
  @apply bg-blue-600/10 text-blue-400 border-blue-600/30;
}

.nav-item--inactive {
  @apply text-gray-400 hover:text-white hover:bg-gray-700 border-transparent;
}

.nav-icon {
  @apply text-base;
}

.nav-icon--active {
  @apply grayscale-0 brightness-100;
}

.nav-icon--inactive {
  @apply grayscale brightness-50;
}

.settings-content {
  @apply flex-1 min-w-0;
}

.loading-card {
  @apply bg-gray-800 rounded-lg border border-gray-700 p-6 shadow-xl flex justify-center py-20;
}

.spinner {
  @apply animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500;
}

.tab-label {
  @apply capitalize;
}

.config-card {
  @apply bg-gray-800 rounded-lg border border-gray-700 p-6 shadow-xl animate-in fade-in duration-300;
}

.config-header {
  @apply text-xl font-bold text-white mb-6 border-b border-gray-700 pb-3 capitalize;
}

.form-section {
  @apply space-y-6;
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
  @apply w-full bg-gray-900 border border-gray-700 rounded-md px-3 py-2.5 text-white transition-all 
         focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}

.form-divider {
  @apply h-px bg-gray-700 my-4 mx-2 opacity-10;
}

.section-spacer {
  @apply h-6;
}

.form-actions {
  @apply pt-6 border-t border-gray-700 flex justify-end;
}

.btn-submit {
  @apply bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white px-6 py-2.5 
         rounded-md font-bold transition-all shadow-lg hover:shadow-blue-600/20 active:scale-95;
}
</style>
