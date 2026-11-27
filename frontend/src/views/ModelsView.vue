<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue"
import { useRoute } from "vue-router"
import { useModels } from "../composables/models/useModels"
import { useConfig } from "../composables/models/useConfig"
import { useProviders } from "../composables/models/useProviders"
import { useProviderKeys } from "../composables/settings/useProviderKeys"
import { useConfirm } from "../composables/ui/useConfirm"
import { useResponsiveLayout } from "../composables/ui/useResponsiveLayout"
import { useToast } from "../composables/useToast"
import { useUnsavedChangesGuard } from "../composables/ui/useUnsavedChangesGuard"
import { toModels, toModelsProvider } from "../router/routes"
import { railEntries } from "../domain/models"
import { MODELS_PROVIDERS_SEGMENT, MODELS_TABS, ROUTE_NAMES } from "../types/routes"
import { errorMessage } from "../utils/errors"
import type { GlobalConfig, ProviderType } from "../types/admin"
import type { Model, ModelEditorOpen, RailId } from "../types/model"
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
import ProviderRail from "../components/models/ProviderRail.vue"
import ProviderPane from "../components/models/ProviderPane.vue"
import ModelEngineSettings from "../components/models/ModelEngineSettings.vue"
import ModelRoutingSettings from "../components/models/ModelRoutingSettings.vue"
import ModelDiscoveredFiles from "../components/models/ModelDiscoveredFiles.vue"

// Models: the one place for everything about models (Constitution III.8 as amended 2026-11-26; ADR-0090, ADR-0091).
// The Models tab is a rail of providers beside a pane: All models (cards or a list, edited in a side panel), This computer (the local models and the
// files found on disk), or one cloud provider (its keys and, under each key, its models). On a phone the rail is a screen of its own that a
// Providers row opens. Then the local engine and routing. What is read about them (the running model, calls, speed) is on the dashboard's
// LLM and host tab, as for a stack (ADR-0089).
// Keys and models save at once through their own APIs; the Engine, Routing and Vertex AI fields edit the shared configuration draft,
// saved with the row under them.

type PageTab = "models" | ModelsTab
const TAB_LABELS: Record<PageTab, string> = { models: "Models", engine: "Engine", routing: "Routing" }
const LOCAL: ProviderType = "local"
const REMOVE_ALL_PROMPT = (label: string) => ({
  title: `Remove all ${label} models?`,
  message: "Their settings are removed too; the provider's keys stay.",
  type: "error",
  confirmText: "Remove all",
  cancelText: "Cancel",
} as const)

const REMOVE_KEYS_PROMPT = (label: string) => ({
  title: `Remove all keys of ${label}?`,
  message: "Every key of this provider is deleted, with the models that use them.",
  type: "error",
  confirmText: "Remove all keys",
  cancelText: "Cancel",
} as const)

const route = useRoute()
const { isMobile } = useResponsiveLayout()
// The tab segment is Engine, Routing, or "providers" (part of the Models tab, followed by one provider).
const tab = computed<PageTab>(() => {
  const t = route.params.tab
  return typeof t === "string" && (MODELS_TABS as readonly string[]).includes(t) ? (t as ModelsTab) : "models"
})
const tabs: readonly StackTabItem[] = (["models", ...MODELS_TABS] as const).map((id) => ({ id, label: TAB_LABELS[id], to: toModels(id === "models" ? undefined : id) }))

const { state, error, availableModels, refresh, startModel, stopModel, removeAllModels } = useModels()
const { config, isLoading: configLoading, isSaving, isDirty, error: configError, fetchConfig, updateConfig, discardChanges, ensureProvider } = useConfig()
const { cloudProviders, manifests, getLabel, fetchManifests } = useProviders()
const toast = useToast()
const { confirm } = useConfirm()

const models = computed<Model[]>(() => state.value?.models ?? [])
const providers = computed(() => cloudProviders.value.map((id) => ({ id, label: getLabel(id) })))
const providerLabel = (p: ProviderType) => (p === LOCAL ? "Local" : getLabel(p))

// API keys, per cloud provider: the Providers tab edits them and the editor offers them.
const providerHasBaseUrl = (p: ProviderType) => config.value?.provider_defaults?.[p]?.supports_base_url ?? false
const providerKeys = useProviderKeys(refresh, (p) => config.value.providers?.[p]?.base_url)
const apiKeysOf = (p: ProviderType) => providerKeys.keys.value[p] ?? []
// The provider's own endpoint, from its manifest; a provider without one gets no suggestion rather than another provider's.
const baseUrlOf = (p: ProviderType) => manifests.value.find((m) => m.id === p)?.default_base_url || undefined

// Which pane the Models tab shows: all models (also the provider list on a desktop, where the rail is always there), This computer, or one
// provider. On a phone the list is a screen of its own and a provider has a way back to it.
// By name, so the rail does not reorder itself while the manifests and keys load.
const byLabel = computed(() => [...providers.value].sort((a, b) => a.label.localeCompare(b.label)))
const rail = computed(() => railEntries(byLabel.value, models.value, (p) => apiKeysOf(p).length))
const connectedCount = computed(() => providers.value.filter((p) => apiKeysOf(p.id).length > 0).length)
const selected = computed<RailId>(() => {
  const id = route.params.provider
  return typeof id === "string" && rail.value.some((e) => e.id === id) ? (id as RailId) : "all"
})
const onProviderList = computed(() => route.params.tab === MODELS_PROVIDERS_SEGMENT && selected.value === "all")
const showRail = computed(() => !isMobile.value || onProviderList.value)
const showPane = computed(() => !isMobile.value || !onProviderList.value)
const cloudProvider = computed<ProviderType | null>(() => (selected.value === "all" || selected.value === LOCAL ? null : selected.value))
const localModels = computed(() => models.value.filter((m) => m.provider === LOCAL))
const showSave = computed(() => tab.value !== "models" || cloudProvider.value === "gemini")

// The side panel.
const editor = ref<ModelEditorOpen | null>(null)
const addModel = (provider: ProviderType = LOCAL, key?: string) => (editor.value = { kind: "add", provider, key })

async function removeProviderModels(provider: ProviderType) {
  if (!(await confirm(REMOVE_ALL_PROMPT(providerLabel(provider))))) return
  await removeAllModels(provider)
}

async function removeProviderKeys(provider: ProviderType) {
  if (!(await confirm(REMOVE_KEYS_PROMPT(providerLabel(provider))))) return
  await providerKeys.clearAll(provider)
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
      <div v-if="tab === 'models'" :class="showRail && showPane ? 'grid grid-cols-[220px_minmax(0,1fr)] items-start gap-4' : 'flex flex-col gap-4'">
        <ProviderRail v-if="showRail" :entries="rail" :selected="selected" />
        <div v-if="showPane" class="flex min-w-0 flex-col gap-4">
          <RouterLink v-if="isMobile && selected !== 'all'" :to="toModelsProvider()" class="inline-flex min-h-11 items-center text-accent-info-text hover:underline" data-test="back-to-providers">‹ Providers</RouterLink>
          <RouterLink
            v-else-if="isMobile"
            :to="toModelsProvider()"
            class="flex min-h-11 items-center gap-3 border border-hairline bg-surface px-3 text-primary no-underline hover:bg-surface-hover"
            data-test="providers-link"
          >
            <span class="flex-1">Providers</span>
            <span class="font-mono text-[length:var(--text-micro)] text-muted">{{ connectedCount }} of {{ providers.length }} connected</span>
            <span class="text-faint" aria-hidden="true">›</span>
          </RouterLink>

          <template v-if="selected === 'all'">
            <EmptyState v-if="!models.length" title="No models yet" body="Add a GGUF file from the model directory, or connect a provider and add one of its models.">
              <template #action>
                <div class="flex flex-wrap gap-2">
                  <BaseButton variant="primary" icon="plus" @click="addModel()">Add a local model</BaseButton>
                  <RouterLink :to="toModelsProvider()" class="inline-flex min-h-8 items-center text-accent-info-text hover:underline">Connect a provider</RouterLink>
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

          <section v-else-if="selected === LOCAL" data-test="provider-pane" class="flex min-w-0 flex-col gap-4">
            <EmptyState v-if="!localModels.length" title="No local model yet" body="Add a GGUF file found in the model directory below, or add one by its file name.">
              <template #action><BaseButton variant="primary" icon="plus" @click="addModel()">Add a local model</BaseButton></template>
            </EmptyState>
            <ModelsCatalog
              v-else
              :models="localModels"
              :config="state.config"
              :active="state.active"
              :provider-label="providerLabel"
              hide-scope
              @edit="editor = { kind: 'edit', model: $event }"
              @start="startModel"
              @stop="stopModel"
            />
            <ModelDiscoveredFiles :files="availableModels" :models="models" @add="editor = { kind: 'file', file: $event }" @rescan="refresh" />
            <p class="m-0 text-[length:var(--text-small)] text-muted">
              The model folder, the llama-server and its arguments are on the
              <RouterLink :to="toModels('engine')" class="text-accent-info-text hover:underline">Engine tab</RouterLink>.
            </p>
          </section>

          <ErrorState v-else-if="configError && !configLoading" title="Could not load the model settings" :cause="configError" next="Check that the server is running and settings.yml is readable, then retry.">
            <template #action><BaseButton variant="secondary" icon="refresh" @click="fetchConfig">Retry</BaseButton></template>
          </ErrorState>
          <LoadingState v-else-if="configLoading" label="Loading model settings" />
          <ProviderPane
            v-else-if="cloudProvider"
            :provider="cloudProvider"
            :label="getLabel(cloudProvider)"
            :keys="apiKeysOf(cloudProvider)"
            :test-status="providerKeys.testStatus.value[cloudProvider]"
            :models="models.filter((m) => m.provider === cloudProvider)"
            :edit-config="config"
            :show-base-url="providerHasBaseUrl(cloudProvider)"
            :base-url-placeholder="baseUrlOf(cloudProvider)"
            @save-keys="providerKeys.save(cloudProvider, $event)"
            @test-key="providerKeys.test(cloudProvider, $event)"
            @clear-test="providerKeys.clearTest(cloudProvider)"
            @add-model="addModel(cloudProvider, $event)"
            @edit-model="editor = { kind: 'edit', model: $event }"
            @remove-models="removeProviderModels(cloudProvider)"
            @remove-keys="removeProviderKeys(cloudProvider)"
            @update:edit-config="updateConfigValue"
          />
          <SettingsActions v-if="showSave && !configLoading && !configError" :dirty="isDirty" :saving="isSaving" :error="saveError" save-label="Save model settings" @save="save" @discard="discard" />
        </div>
      </div>

      <ErrorState v-else-if="configError && !configLoading" title="Could not load the model settings" :cause="configError" next="Check that the server is running and settings.yml is readable, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" @click="fetchConfig">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="configLoading" label="Loading model settings" />

      <template v-else>
        <template v-if="tab === 'engine'">
          <ModelEngineSettings :edit-config="config" @update:edit-config="updateConfigValue" />
          <InfrastructurePanel />
        </template>
        <ModelRoutingSettings v-else :edit-config="config" :models="models" @update:edit-config="updateConfigValue" />
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
