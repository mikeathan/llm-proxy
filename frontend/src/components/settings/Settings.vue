<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import GlobalSettings from "./GlobalSettings.vue";
import McpServers from "./McpServers.vue";
import ApiKeySettings from "./ApiKeySettings.vue";
import GuardrailSettings from "./GuardrailSettings.vue";
import { useConfig } from "../../composables/useConfig";
import { useMcpServers } from "../../composables/useMcpServers";
import { useMetrics } from "../../composables/useMetrics";
import { useModels } from "../../composables/useModels";
import { AdminApiService } from "../../services/adminService";
import { useProviders } from "../../composables/useProviders";
import { useToast } from "../../composables/useToast";
import type { NewMcpServerForm } from "../../types/mcp";
import type { ProviderType, APIKeyItem, SettingsTab } from "../../types/admin";
import { generateId } from "../../utils/crypto";
import { isProviderTab, getSettingsGroups } from "../../domain/settings";


const {
  config,
  updateConfig,
  fetchConfig,
  isSaving,
  isLoading,
  ensureProvider,
} = useConfig();
const { state: adminModelsState, refresh: refreshModels } = useModels();
const modelsList = computed(() => adminModelsState.value?.models || []);
const { mcpServers, addMCPServer, toggleMCPServer, removeMCPServer } =
  useMcpServers();
const { settingsTabs, getIcon, getLabel, fetchManifests, cloudProviders } =
  useProviders();
const { logLevel, updateLogLevel } = useMetrics();
const toast = useToast();

const activeTab = ref<SettingsTab>("local");
const testStatus = ref<{
  [key: string]: { loading: boolean; error?: string; success?: string };
}>({});

// Dynamic key list computed mapping
const providerKeys = computed(() => {
  const map: Record<string, APIKeyItem[]> = {};
  cloudProviders.value.forEach((p) => {
    map[p] = ensureIds(config.value.providers?.[p]?.api_keys ?? []);
  });
  return map;
});

function setTab(tab: SettingsTab) {
  activeTab.value = tab;
  // Ensure provider exists so fields can bind to config.providers[type]
  if (isProviderTab(tab)) {
    ensureProvider(tab);
  }
}


function ensureIds(keys: any[]): APIKeyItem[] {
  return keys.map((k) => {
    if (k.id) return k;
    return {
      ...k,
      id: generateId(),
    };
  });
}


async function updateApiKeys(type: ProviderType, keys: APIKeyItem[]) {
  const provider = ensureProvider(type);
  provider.api_keys = ensureIds(keys);
  try {
    await AdminApiService.updateConfig(
      JSON.parse(JSON.stringify(config.value)),
    );
    await refreshModels();
  } catch (e: any) {
    console.error(`[Settings] Failed to auto-save ${type} API keys:`, e);
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

const testProvider = async (type: string, apiKey?: string) => {
  testStatus.value[type] = { loading: true };
  try {
    const res = await AdminApiService.testConnection(type, apiKey);
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

function clearTestStatus(type: string) {
  testStatus.value[type] = { loading: false };
}

const newMcpServer = ref<NewMcpServerForm>({ name: "", url: "" });
const handleAddMCPServer = (): void => {
  if (!newMcpServer.value.name || !newMcpServer.value.url) return;
  addMCPServer(newMcpServer.value);
  newMcpServer.value = { name: "", url: "" };
};

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
        <h3 v-if="group.tabs.length > 0" class="group-title">{{ group.name }}</h3>
        <nav class="nav-list">
          <button
            v-for="tab in group.tabs"
            :key="tab"
            @click="setTab(tab)"
            class="nav-item"
            :class="activeTab === tab ? 'nav-item--active' : 'nav-item--inactive'"
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
          />
        </div>

        <!-- Guardrails -->
        <div v-show="activeTab === 'guardrails'">
          <GuardrailSettings
            v-model:config="config"
            @save="handleSaveConfig"
          />
        </div>

        <!-- Provider Configs -->
        <div
          v-for="provider in settingsTabs.filter(isProviderTab) as ProviderType[]"
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
                :testLoading="!!testStatus[provider]?.loading"
                :testSuccess="testStatus[provider]?.success"
                :testError="testStatus[provider]?.error"
                @update:apiKeys="updateApiKeys(provider, $event)"
                @testKey="testProvider(provider, $event)"
                @clearTest="clearTestStatus(provider)"
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

              <template
                v-if="provider === 'openai' && config.providers?.openai"
              >
                <div class="form-group">
                  <label class="form-label"
                    >Base URL
                    <span class="form-optional">(Optional)</span></label
                  >
                  <div class="form-helper">
                    Override for localized proxies or self-hosted engines
                  </div>
                  <input
                    v-model="config.providers.openai.base_url"
                    type="text"
                    placeholder="https://api.openai.com/v1"
                    class="form-input"
                  />
                </div>
              </template>

              <template
                v-if="provider === 'mulerouter' && config.providers?.mulerouter"
              >
                <div class="form-group">
                  <label class="form-label"
                    >Base URL
                    <span class="form-optional">(Optional)</span></label
                  >
                  <div class="form-helper">
                    Default: https://api.mulerouter.ai/v1
                  </div>
                  <input
                    v-model="config.providers.mulerouter.base_url"
                    type="text"
                    placeholder="https://api.mulerouter.ai/v1"
                    class="form-input"
                  />
                </div>
              </template>

              <template
                v-if="provider === 'nvidia' && config.providers?.nvidia"
              >
                <div class="form-group">
                  <label class="form-label"
                    >Base URL
                    <span class="form-optional">(Optional)</span></label
                  >
                  <div class="form-helper">
                    Default: https://integrate.api.nvidia.com/v1
                  </div>
                  <input
                    v-model="config.providers.nvidia.base_url"
                    type="text"
                    placeholder="https://integrate.api.nvidia.com/v1"
                    class="form-input"
                  />
                </div>
              </template>

              <div class="form-actions">
                <button type="submit" class="btn-submit" :disabled="isSaving">
                  {{ isSaving ? "Saving..." : `Save ${provider} Config` }}
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

.form-actions {
  @apply pt-6 border-t border-gray-700 flex justify-end;
}

.btn-submit {
  @apply bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white px-6 py-2.5 
         rounded-md font-bold transition-all shadow-lg hover:shadow-blue-600/20 active:scale-95;
}
</style>
