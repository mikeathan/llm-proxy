<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from "vue"
import { useConfig } from "../composables/models/useConfig"
import { useLogLevel } from "../composables/system/useMetrics"
import { useToast } from "../composables/useToast"
import { useConfirm } from "../composables/ui/useConfirm"
import { useUnsavedChangesGuard } from "../composables/ui/useUnsavedChangesGuard"
import { useSettingsSection } from "../composables/settings/useSettingsSection"
import { AdminApiService } from "../services/admin/adminService"
import { getSettingsTabGroups, isSettingsSection } from "../domain/settings"
import { GENERAL_PANEL_SECTION, SETTINGS_SECTION_LABELS } from "../constants/settings"
import { errorMessage } from "../utils/errors"
import { ROUTE_NAMES } from "../types/routes"
import type { GlobalConfig, SettingsTab } from "../types/admin"
import PageHeader from "../components/common/layout/PageHeader.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import UnsavedTag from "../components/common/display/UnsavedTag.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import SettingsTabs from "../components/settings/SettingsTabs.vue"
import SettingsActions from "../components/settings/SettingsActions.vue"
import GlobalSettings from "../components/settings/GlobalSettings.vue"
import SecuritySettings from "../components/settings/SecuritySettings.vue"
import GuardrailSettings from "../components/settings/GuardrailSettings.vue"
import CommunicationSettings from "../components/settings/CommunicationSettings.vue"
import AgentsSettings from "../components/settings/AgentsSettings.vue"
import SignInSettings from "../components/settings/SignInSettings.vue"
import SearchSettings from "../components/settings/SearchSettings.vue"
import AppearanceSettings from "../components/settings/AppearanceSettings.vue"

// Settings · global (plan Phase 5): one route per category (/settings/:section),
// the long forms split into numbered panels, and one save model for the shared
// configuration — edits survive switching categories, every editable section
// ends with the same Discard / Save row, and leaving with unsaved edits asks
// first. Everything about models is on the Models page.

// The backend exits and comes back; the page reloads once it has had time to.
const RESTART_RELOAD_MS = 5000
const RESTART_PROMPT = {
  title: "Restart the backend?",
  message: "Running models, terminal sessions and automations stop. The page reloads in about 5 seconds.",
  type: "warning",
  confirmText: "Restart",
} as const
// Sections with a draft of their own (not part of the shared configuration).
type DraftSection = "appearance" | "security" | "communication" | "search" | "agents"

const { config, isLoading, isSaving, isDirty: configDirty, error: configError, fetchConfig, updateConfig, discardChanges } = useConfig()
const { logLevel, updateLogLevel } = useLogLevel()
const toast = useToast()
const { confirm } = useConfirm()

// ── Sections ──────────────────────────────────────────────────────────────
const settingsGroups = getSettingsTabGroups()
const { activeTab } = useSettingsSection(isSettingsSection, ref(true))
const labelOf = (tab: SettingsTab) => SETTINGS_SECTION_LABELS[tab]
// General is several short sections over one form: all stay mounted, so an edit survives moving between them.
const isGeneralSection = computed(() => Object.values(GENERAL_PANEL_SECTION).includes(activeTab.value))
// Security is two sections over one draft, for the same reason.
const isSecuritySection = computed(() => activeTab.value === 'security' || activeTab.value === 'limits')

// ── Saving the shared configuration ───────────────────────────────────────
const saveError = ref("")
const loadFailed = ref(false)
const draftDirty = reactive<Record<DraftSection, boolean>>({ appearance: false, security: false, communication: false, search: false, agents: false })
const anyDirty = computed(() => configDirty.value || Object.values(draftDirty).some(Boolean))

async function saveConfig() {
  saveError.value = ""
  try {
    await updateConfig()
    toast.success("Settings saved")
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
}

onMounted(load)
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

    <div class="flex min-w-0 flex-col gap-4">
      <SettingsTabs :groups="settingsGroups" :active="activeTab" :label-of="labelOf" />

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

          <div v-show="isGeneralSection" class="flex flex-col gap-4">
            <GlobalSettings
              :section="activeTab"
              :edit-config="config"
              :log-level="logLevel"
              @update:edit-config="updateConfigValue"
              @update-config="saveConfig"
              @update-log-level="updateLogLevel"
            />
            <SettingsActions :dirty="configDirty" :saving="isSaving" :error="saveError" save-label="Save settings" @save="saveConfig" @discard="discardConfig" />
          </div>

          <div v-show="activeTab === 'agents'">
            <AgentsSettings @dirty-change="draftDirty.agents = $event" />
          </div>

          <div v-show="activeTab === 'sign-in'">
            <SignInSettings />
          </div>

          <div v-show="isSecuritySection">
            <SecuritySettings :section="activeTab" @dirty-change="draftDirty.security = $event" />
          </div>

          <div v-show="activeTab === 'guardrails'" class="flex flex-col gap-4">
            <GuardrailSettings :config="config" @update:config="updateConfigValue" />
            <SettingsActions :dirty="configDirty" :saving="isSaving" :error="saveError" save-label="Save guardrails" @save="saveConfig" @discard="discardConfig" />
          </div>

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

        </template>
      </div>
    </div>
  </div>
</template>
