<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from "vue"
import { useConfig } from "../composables/models/useConfig"
import { useModels } from "../composables/models/useModels"
import { useProviders } from "../composables/models/useProviders"
import { useMcpServers } from "../composables/system/useMcpServers"
import { useLogLevel } from "../composables/system/useMetrics"
import { useToast } from "../composables/useToast"
import { useConfirm } from "../composables/ui/useConfirm"
import { useUnsavedChangesGuard } from "../composables/ui/useUnsavedChangesGuard"
import { useSettingsSection } from "../composables/settings/useSettingsSection"
import { useProviderKeys } from "../composables/settings/useProviderKeys"
import { AdminApiService } from "../services/admin/adminService"
import { getSettingsGroups, isProviderTab } from "../domain/settings"
import { errorMessage } from "../utils/errors"
import { ROUTE_NAMES } from "../types/routes"
import type { GlobalConfig, ProviderType, SettingsTab } from "../types/admin"
import type { Model } from "../types/model"
import type { NewMcpServerForm } from "../types/mcp"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import UnsavedTag from "../components/common/display/UnsavedTag.vue"
import FormField from "../components/common/forms/FormField.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import SettingsNav from "../components/settings/SettingsNav.vue"
import SettingsActions from "../components/settings/SettingsActions.vue"
import GlobalSettings from "../components/settings/GlobalSettings.vue"
import SecuritySettings from "../components/settings/SecuritySettings.vue"
import GuardrailSettings from "../components/settings/GuardrailSettings.vue"
import McpServers from "../components/settings/McpServers.vue"
import ApiKeySettings from "../components/settings/ApiKeySettings.vue"
import ProviderModelsCard from "../components/settings/ProviderModelsCard.vue"
import CommunicationSettings from "../components/settings/CommunicationSettings.vue"
import SearchSettings from "../components/settings/SearchSettings.vue"
import InfrastructurePanel from "../components/infrastructure/InfrastructurePanel.vue"
import AppearanceSettings from "../components/settings/AppearanceSettings.vue"

// Settings · global (plan Phase 5): one route per category (/settings/:section),
// the long forms split into numbered panels, and one save model for the shared
// configuration — edits survive switching categories, every editable section
// ends with the same Discard / Save row, and leaving with unsaved edits asks
// first. Keys, models, MCP servers and processes act immediately (their own
// APIs), so those sections have no Save.

const LOCAL_PROVIDER = "local"
// The backend exits and comes back; the page reloads once it has had time to.
const RESTART_RELOAD_MS = 5000
const RESTART_PROMPT = {
  title: "Restart the backend?",
  message: "Running models, terminal sessions and automations stop. The page reloads in about 5 seconds.",
  type: "warning",
  confirmText: "Restart",
} as const
// Sections with a draft of their own (not part of the shared configuration).
type DraftSection = "appearance" | "security" | "communication" | "search"

const { config, isLoading, isSaving, isDirty: configDirty, error: configError, fetchConfig, updateConfig, discardChanges, ensureProvider } = useConfig()
const { state: modelsState, availableModels, refresh: refreshModels } = useModels()
const { settingsTabs, getLabel, fetchManifests } = useProviders()
const { mcpServers, addMCPServer, toggleMCPServer, removeMCPServer } = useMcpServers()
const { logLevel, updateLogLevel } = useLogLevel()
const toast = useToast()
const { confirm } = useConfirm()

// ── Sections ──────────────────────────────────────────────────────────────
const manifestsLoaded = ref(false)
const settingsGroups = computed(() => getSettingsGroups(settingsTabs.value))
const { activeTab } = useSettingsSection(
  (tab) => settingsGroups.value.some((group) => group.tabs.includes(tab)),
  manifestsLoaded,
)
const providerTabs = computed(() => settingsTabs.value.filter(isProviderTab) as ProviderType[])
const labelOf = (tab: SettingsTab) => getLabel(tab)

// ── Models ────────────────────────────────────────────────────────────────
const models = computed<Model[]>(() => modelsState.value?.models ?? [])
const modelsOf = (provider: string) => models.value.filter((m) => m.provider === provider)
function modelCountsByKey(provider: string): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const m of modelsOf(provider)) {
    const key = m.provider_config?.api_key_name || ""
    counts[key] = (counts[key] || 0) + 1
  }
  return counts
}

// ── Provider keys ─────────────────────────────────────────────────────────
// Whether a provider takes a per-key base URL is a backend capability.
const providerHasBaseUrl = (p: string) => config.value?.provider_defaults?.[p]?.supports_base_url ?? false
const providerKeys = useProviderKeys(refreshModels, (p) => config.value.providers?.[p]?.base_url)

// A provider section can be the one the page opens on (a deep link), so this
// runs immediately as well as on every section change.
watch(
  activeTab,
  (tab) => {
    if (!isProviderTab(tab)) return
    ensureProvider(tab)
    void providerKeys.load(tab as ProviderType)
  },
  { immediate: true },
)

// ── Saving the shared configuration ───────────────────────────────────────
const saveError = ref("")
const loadFailed = ref(false)
const draftDirty = reactive<Record<DraftSection, boolean>>({ appearance: false, security: false, communication: false, search: false })
const anyDirty = computed(() => configDirty.value || Object.values(draftDirty).some(Boolean))

async function saveConfig() {
  saveError.value = ""
  try {
    await updateConfig()
    toast.success("Settings saved")
    await refreshModels()
  } catch (e) {
    saveError.value = `Could not save settings: ${errorMessage(e)}. Your changes are still here — try again.`
  }
}

function discardConfig() {
  saveError.value = ""
  discardChanges()
}

// Switching category keeps every edit (all sections stay mounted), so only
// leaving Settings can lose them.
useUnsavedChangesGuard(anyDirty, { discards: (to) => to.name !== ROUTE_NAMES.settings })

// ── MCP servers ───────────────────────────────────────────────────────────
const newMcpServer = ref<NewMcpServerForm>({ name: "", url: "" })
function handleAddMCPServer() {
  if (!newMcpServer.value.name || !newMcpServer.value.url) return
  addMCPServer(newMcpServer.value)
  newMcpServer.value = { name: "", url: "" }
}

// ── Restart ───────────────────────────────────────────────────────────────
async function restartBackend() {
  if (!(await confirm(RESTART_PROMPT))) return
  try {
    await AdminApiService.restartSystem()
    toast.info("Restart requested. Reconnecting in about 5 seconds…")
    setTimeout(() => window.location.reload(), RESTART_RELOAD_MS)
  } catch (e) {
    toast.error(`Could not restart the backend: ${errorMessage(e)}`)
  }
}

async function load() {
  await fetchConfig()
  loadFailed.value = !!configError.value
  // The loaded config replaces the one the section watch prepared.
  if (isProviderTab(activeTab.value)) ensureProvider(activeTab.value)
}

onMounted(async () => {
  void load()
  void refreshModels()
  await fetchManifests()
  manifestsLoaded.value = true
})
// The configuration is shared app state: edits abandoned by leaving (the
// guard asked) must not linger for the next visit or another reader.
onUnmounted(discardChanges)

const updateConfigValue = (value: GlobalConfig) => {
  config.value = value
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="Configuration" title="Settings" subtitle="Global configuration · workspace settings live under each workspace">
      <template #actions>
        <UnsavedTag v-if="anyDirty" />
        <BaseButton variant="secondary" icon="refresh" @click="restartBackend">Restart backend</BaseButton>
      </template>
    </PageHeader>

    <div class="grid items-start gap-3 lg:grid-cols-[200px_minmax(0,1fr)] lg:gap-6">
      <SettingsNav :groups="settingsGroups" :active="activeTab" :label-of="labelOf" />

      <div class="flex min-w-0 flex-col gap-4">
        <ErrorState
          v-if="loadFailed"
          title="Could not load settings"
          :cause="configError ?? ''"
          next="Check that the server is running and settings.yml is readable, then retry."
        >
          <template #action><BaseButton variant="secondary" icon="refresh" @click="load">Retry</BaseButton></template>
        </ErrorState>
        <LoadingState v-else-if="isLoading" label="Loading settings" />

        <template v-else>
          <div v-show="activeTab === 'appearance'">
            <AppearanceSettings @dirty-change="draftDirty.appearance = $event" />
          </div>

          <div v-show="activeTab === 'local'" class="flex flex-col gap-4">
            <GlobalSettings
              :edit-config="config"
              :log-level="logLevel"
              :models="models"
              @update:edit-config="updateConfigValue"
              @update-config="saveConfig"
              @update-log-level="updateLogLevel"
            />
            <SettingsActions :dirty="configDirty" :saving="isSaving" :error="saveError" save-label="Save settings" @save="saveConfig" @discard="discardConfig" />
          </div>

          <div v-show="activeTab === 'local-models'">
            <ProviderModelsCard :provider="LOCAL_PROVIDER" :api-keys="[]" :models="modelsOf(LOCAL_PROVIDER)" :available-models="availableModels" @refresh="refreshModels" />
          </div>

          <div v-show="activeTab === 'security'">
            <SecuritySettings :active="activeTab === 'security'" @dirty-change="draftDirty.security = $event" />
          </div>

          <div v-show="activeTab === 'guardrails'" class="flex flex-col gap-4">
            <GuardrailSettings :config="config" @update:config="updateConfigValue" />
            <SettingsActions :dirty="configDirty" :saving="isSaving" :error="saveError" save-label="Save guardrails" @save="saveConfig" @discard="discardConfig" />
          </div>

          <!-- Polls while showing; nothing to keep when switching away. -->
          <InfrastructurePanel v-if="activeTab === 'processes'" />

          <div v-show="activeTab === 'communication'">
            <CommunicationSettings
              :edit-config="config"
              :config-dirty="configDirty"
              :saving="isSaving"
              :config-error="saveError"
              @update:edit-config="updateConfigValue"
              @update-config="saveConfig"
              @discard="discardConfig"
              @dirty-change="draftDirty.communication = $event"
            />
          </div>

          <div v-show="activeTab === 'search'">
            <SearchSettings
              :edit-config="config"
              :config-dirty="configDirty"
              :saving="isSaving"
              :config-error="saveError"
              @update:edit-config="updateConfigValue"
              @update-config="saveConfig"
              @discard="discardConfig"
              @dirty-change="draftDirty.search = $event"
            />
          </div>

          <div v-for="provider in providerTabs" v-show="activeTab === provider" :key="provider" class="flex flex-col gap-4">
            <Panel :title="`${labelOf(provider)} · API keys`">
              <ApiKeySettings
                :api-keys="providerKeys.keys.value[provider] ?? []"
                :show-base-url="providerHasBaseUrl(provider)"
                :test-loading="!!providerKeys.testStatus.value[provider]?.loading"
                :test-success="providerKeys.testStatus.value[provider]?.success"
                :test-error="providerKeys.testStatus.value[provider]?.error"
                :model-counts="modelCountsByKey(provider)"
                @update:api-keys="providerKeys.save(provider, $event)"
                @test-key="providerKeys.test(provider, $event)"
                @clear-test="providerKeys.clearTest(provider)"
                @clear-all="providerKeys.clearAll(provider)"
              />
            </Panel>

            <ProviderModelsCard :provider="provider" :api-keys="providerKeys.keys.value[provider] ?? []" :models="modelsOf(provider)" @refresh="refreshModels" />

            <template v-if="provider === 'gemini' && config.providers?.gemini">
              <Panel title="Vertex AI">
                <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
                  <FormField label="Project ID" hint="Optional. Required only for Vertex AI.">
                    <template #default="{ id, describedBy }">
                      <input :id="id" v-model="config.providers.gemini.project_id" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
                    </template>
                  </FormField>
                  <FormField label="Region" hint="Optional. The Vertex AI region, such as us-central1.">
                    <template #default="{ id, describedBy }">
                      <input :id="id" v-model="config.providers.gemini.region" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
                    </template>
                  </FormField>
                </div>
              </Panel>
              <SettingsActions :dirty="configDirty" :saving="isSaving" :error="saveError" save-label="Save Vertex AI settings" @save="saveConfig" @discard="discardConfig" />
            </template>
          </div>

          <div v-show="activeTab === 'mcp'">
            <McpServers
              v-model:new-mcp-server="newMcpServer"
              :mcp-servers="mcpServers"
              @add="handleAddMCPServer"
              @toggle="toggleMCPServer"
              @remove="removeMCPServer"
            />
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
