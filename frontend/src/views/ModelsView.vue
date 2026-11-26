<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue"
import { useRoute } from "vue-router"
import { useModels } from "../composables/models/useModels"
import { useConfig } from "../composables/models/useConfig"
import { useProviders } from "../composables/models/useProviders"
import { useProviderKeys } from "../composables/settings/useProviderKeys"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import { useUnsavedChangesGuard } from "../composables/ui/useUnsavedChangesGuard"
import { toModels } from "../router/routes"
import { MODELS_TABS, ROUTE_NAMES } from "../types/routes"
import { errorMessage } from "../utils/errors"
import type { GlobalConfig, ProviderType } from "../types/admin"
import type { Model, ModelEditorOpen } from "../types/model"
import type { ModelsTab } from "../types/routes"
import type { StackTabItem } from "../types/stacks"
import PageHeader from "../components/common/layout/PageHeader.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import SettingsActions from "../components/settings/SettingsActions.vue"
import InfrastructurePanel from "../components/infrastructure/InfrastructurePanel.vue"
import ModelsCatalog from "../components/models/ModelsCatalog.vue"
import ModelEditorPanel from "../components/models/ModelEditorPanel.vue"
import ModelProviders from "../components/models/ModelProviders.vue"
import ModelEngineSettings from "../components/models/ModelEngineSettings.vue"
import ModelRoutingSettings from "../components/models/ModelRoutingSettings.vue"
import ModelDiscoveredFiles from "../components/models/ModelDiscoveredFiles.vue"

// Models: the one place for everything about models (Constitution III.8 as amended 2026-11-26; ADR-0090).
// The models themselves (cards or a list, edited in a side panel), the cloud providers and their keys, the local engine, and routing.
// What is read about them (the running model, calls, speed) is on the dashboard's LLM and host tab, as for a stack (ADR-0089).
// Keys and models save at once through their own APIs; the Engine, Routing and Vertex AI fields edit the shared configuration draft,
// saved with the row under them.

type PageTab = "models" | ModelsTab
const TAB_LABELS: Record<PageTab, string> = { models: "Models", providers: "Providers", engine: "Engine", routing: "Routing" }
const LOCAL: ProviderType = "local"
const REMOVE_ALL_PROMPT = (label: string) => ({
  title: `Remove all ${label} models?`,
  message: "Their settings are removed too; the provider's keys stay.",
  type: "error",
  confirmText: "Remove all",
  cancelText: "Cancel",
} as const)

const route = useRoute()
const tab = computed<PageTab>(() => {
  const t = route.params.tab
  return typeof t === "string" && (MODELS_TABS as readonly string[]).includes(t) ? (t as ModelsTab) : "models"
})
const tabs: readonly StackTabItem[] = (["models", ...MODELS_TABS] as const).map((id) => ({ id, label: TAB_LABELS[id], to: toModels(id === "models" ? undefined : id) }))

const { state, error, availableModels, refresh, startModel, stopModel, removeAllModels } = useModels()
const { config, isLoading: configLoading, isSaving, isDirty, error: configError, fetchConfig, updateConfig, discardChanges, ensureProvider } = useConfig()
const { cloudProviders, getLabel, fetchManifests } = useProviders()
const toast = useToast()
const { confirm } = useConfirm()

const models = computed<Model[]>(() => state.value?.models ?? [])
const providers = computed(() => cloudProviders.value.map((id) => ({ id, label: getLabel(id) })))
const providerLabel = (p: ProviderType) => (p === LOCAL ? "Local" : getLabel(p))

// API keys, per cloud provider: the Providers tab edits them and the editor offers them.
const providerHasBaseUrl = (p: ProviderType) => config.value?.provider_defaults?.[p]?.supports_base_url ?? false
const providerKeys = useProviderKeys(refresh, (p) => config.value.providers?.[p]?.base_url)
const apiKeysOf = (p: ProviderType) => providerKeys.keys.value[p] ?? []

// The side panel.
const editor = ref<ModelEditorOpen | null>(null)
const addModel = (provider: ProviderType = LOCAL) => (editor.value = { kind: "add", provider })

async function removeProviderModels(provider: ProviderType) {
  if (!(await confirm(REMOVE_ALL_PROMPT(providerLabel(provider))))) return
  await removeAllModels(provider)
}

// The shared configuration draft (Engine, Routing, Vertex AI).
const saveError = ref("")
async function save() {
  saveError.value = ""
  try {
    await updateConfig()
    toast.success("Model settings saved")
    await refresh()
  } catch (e) {
    saveError.value = `Could not save the model settings: ${errorMessage(e)}. Your changes are still here — try again.`
  }
}
function discard() {
  saveError.value = ""
  discardChanges()
}
const updateConfigValue = (value: GlobalConfig) => {
  config.value = value
}
useUnsavedChangesGuard(isDirty, { discards: (to) => to.name !== ROUTE_NAMES.models })

onMounted(async () => {
  void refresh()
  await fetchConfig()
  ensureProvider("gemini")
  await fetchManifests()
  await Promise.all(cloudProviders.value.map((p) => providerKeys.load(p)))
})
// The draft is shared app state: edits abandoned by leaving (the guard asked) must not linger for Settings or the next visit.
onUnmounted(discardChanges)
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader title="Models">
      <template #actions>
        <BaseButton variant="primary" icon="plus" @click="addModel()">Add model</BaseButton>
      </template>
    </PageHeader>
    <StackTabs label="Models" :tabs="tabs" :active="tab" />
    <StackBoardLink stack="models" />

    <ErrorState
      v-if="!state && error"
      title="Could not load the model catalogue"
      :cause="error"
      next="Check that the server is running and its registry is readable, then retry."
    >
      <template #action><BaseButton variant="secondary" icon="refresh" @click="refresh">Retry</BaseButton></template>
    </ErrorState>
    <LoadingState v-else-if="!state" label="Loading models" />

    <template v-else>
      <template v-if="tab === 'models'">
        <EmptyState v-if="!models.length" title="No models yet" body="Add a GGUF file from the model directory, or connect a provider and add one of its models.">
          <template #action>
            <div class="flex flex-wrap gap-2">
              <BaseButton variant="primary" icon="plus" @click="addModel()">Add a local model</BaseButton>
              <RouterLink :to="toModels('providers')" class="inline-flex min-h-8 items-center text-accent-info-text hover:underline">Connect a provider</RouterLink>
            </div>
          </template>
        </EmptyState>
        <ModelsCatalog
          v-else
          :models="models"
          :config="state.config"
          :active="state.active"
          :provider-label="providerLabel"
          @edit="editor = { kind: 'edit', model: $event }"
          @start="startModel"
          @stop="stopModel"
        />
      </template>

      <ErrorState v-else-if="configError && !configLoading" title="Could not load the model settings" :cause="configError" next="Check that the server is running and settings.yml is readable, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" @click="fetchConfig">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="configLoading" label="Loading model settings" />

      <template v-else>
        <ModelProviders
          v-if="tab === 'providers'"
          :providers="providers"
          :keys="providerKeys.keys.value"
          :test-status="providerKeys.testStatus.value"
          :models="models"
          :edit-config="config"
          :show-base-url="providerHasBaseUrl"
          @save-keys="providerKeys.save"
          @test-key="providerKeys.test"
          @clear-test="providerKeys.clearTest"
          @clear-keys="providerKeys.clearAll"
          @add-model="addModel"
          @remove-models="removeProviderModels"
          @update:edit-config="updateConfigValue"
        />
        <template v-else-if="tab === 'engine'">
          <ModelEngineSettings :edit-config="config" @update:edit-config="updateConfigValue" />
          <ModelDiscoveredFiles :files="availableModels" :models="models" @add="editor = { kind: 'file', file: $event }" @rescan="refresh" />
          <InfrastructurePanel />
        </template>
        <ModelRoutingSettings v-else-if="tab === 'routing'" :edit-config="config" :models="models" @update:edit-config="updateConfigValue" />

        <SettingsActions :dirty="isDirty" :saving="isSaving" :error="saveError" save-label="Save model settings" @save="save" @discard="discard" />
      </template>
    </template>

    <ModelEditorPanel
      v-model:open="editor"
      :models="models"
      :available-models="availableModels"
      :providers="providers"
      :api-keys-of="apiKeysOf"
      @refresh="refresh"
    />
  </div>
</template>
