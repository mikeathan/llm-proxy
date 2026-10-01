<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useModels } from "../composables/models/useModels"
import { toSettings } from "../router/routes"
import { PROVIDER_LABELS } from "../constants/providers"
import { formatTokenCount } from "../utils/format/units"
import type { Model } from "../types/model"
import type { SettingsTab } from "../types/admin"
import type { DataTableColumn, StatusState } from "../types/ui"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import DataTable from "../components/common/display/DataTable.vue"
import StatusTag from "../components/common/display/StatusTag.vue"
import IdChip from "../components/common/display/IdChip.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"

// Models (plan D9, Phase 5): local residency and cloud routing. Editing lives
// in Settings (Constitution III.8); this page starts / stops local engines and
// links each model to its settings section.

const LOCAL_PROVIDER = "local"
const LOCAL_SETTINGS: SettingsTab = "local-models"
// Where "connect a provider" lands when there is no cloud model to follow.
const DEFAULT_PROVIDER_SETTINGS: SettingsTab = "openai"
const CREDENTIAL_CHARS = 12
const NO_VALUE = "—"

const { state, error, refresh, startModel, stopModel } = useModels()
onMounted(refresh)

const all = computed(() => state.value?.models ?? [])
const local = computed(() => all.value.filter((m) => m.provider === LOCAL_PROVIDER))
const cloud = computed(() => all.value.filter((m) => m.provider !== LOCAL_PROVIDER))
// One local engine at a time: starting another is blocked while one is active.
const localActive = computed(() => local.value.some((m) => m.active))

const settingsFor = (m: Model) => toSettings(m.provider === LOCAL_PROVIDER ? LOCAL_SETTINGS : (m.provider as SettingsTab))
const providerSettings = computed(() => (cloud.value[0] ? settingsFor(cloud.value[0]) : toSettings(DEFAULT_PROVIDER_SETTINGS)))

function localStatus(m: Model): { state: StatusState; label: string } {
  if (m.active && !m.ready) return { state: "running", label: "Loading" }
  if (m.active) return { state: "success", label: "Active" }
  return { state: "neutral", label: "Idle" }
}

function routingRole(m: Model): string | null {
  const config = state.value?.config
  if (config?.primary_model === m.name) return "Primary"
  if (config?.fallback_model === m.name) return "Fallback"
  return null
}

const modelKey = (m: Model) => `${m.provider}/${m.name}`
const LOCAL_COLUMNS: DataTableColumn<Model>[] = [
  { key: "name", label: "Model" },
  { key: "status", label: "Status" },
  { key: "context", label: "Context", numeric: true, value: (m) => (m.metadata?.context_length ? formatTokenCount(m.metadata.context_length) : NO_VALUE) },
  { key: "port", label: "Port", numeric: true, value: (m) => (m.port ? `:${m.port}` : NO_VALUE) },
  { key: "actions", label: "Actions" },
]
const CLOUD_COLUMNS: DataTableColumn<Model>[] = [
  { key: "name", label: "Model" },
  { key: "provider", label: "Provider", value: (m) => PROVIDER_LABELS[m.provider as SettingsTab] ?? m.provider },
  { key: "routing", label: "Routing" },
  { key: "credential", label: "Credential" },
  { key: "actions", label: "Actions" },
]
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="Routing" title="Models" subtitle="Local residency and cloud routing">
      <template #actions>
        <RouterLink
          :to="toSettings(LOCAL_SETTINGS)"
          class="offset-brand inline-flex h-[30px] items-center gap-1.5 rounded-[var(--radius-sm)] border border-text-primary bg-text-primary px-3 font-mono text-[length:var(--text-small)] font-medium text-inverse focus-visible:outline-none focus-visible:ring-2"
        >+ Add model</RouterLink>
      </template>
    </PageHeader>

    <ErrorState
      v-if="!state && error"
      title="Could not load the model catalogue"
      :cause="error"
      next="Check that the server is running and its registry is readable, then retry."
    >
      <template #action><BaseButton variant="secondary" icon="refresh" @click="refresh">Retry</BaseButton></template>
    </ErrorState>
    <LoadingState v-else-if="!state" label="Loading models" />

    <EmptyState v-else-if="!all.length" title="No models configured" body="Add a local GGUF model or connect a cloud provider to start routing requests.">
      <template #action>
        <div class="flex flex-wrap gap-2">
          <RouterLink :to="toSettings(LOCAL_SETTINGS)" class="settings-link">Add a local model</RouterLink>
          <RouterLink :to="providerSettings" class="settings-link">Connect a provider</RouterLink>
        </div>
      </template>
    </EmptyState>

    <template v-else>
      <Panel title="Local" flush>
        <DataTable
          :columns="LOCAL_COLUMNS"
          :rows="local"
          :row-key="modelKey"
          caption="Local models"
          empty-title="No local models"
          empty-body="Add a GGUF model in Settings to serve it through llama.cpp."
        >
          <template #cell-name="{ row }">
            <span class="font-mono text-primary">{{ row.name }}</span>
            <span v-if="row.metadata?.quantization" class="ml-2 font-mono text-[length:var(--text-small)] text-faint">{{ row.metadata.quantization }}</span>
          </template>
          <template #cell-status="{ row }">
            <StatusTag v-bind="localStatus(row)" />
          </template>
          <template #cell-actions="{ row }">
            <span class="inline-flex items-center gap-1">
              <BaseButton v-if="row.active" variant="ghost" size="sm" icon="stop" icon-only :label="`Stop ${row.name}`" @click="stopModel" />
              <BaseButton
                v-else
                variant="ghost"
                size="sm"
                icon="play"
                icon-only
                :label="`Start ${row.name}`"
                :disabled="localActive"
                @click="startModel(row.name)"
              />
              <RouterLink :to="settingsFor(row)" class="settings-link">Settings</RouterLink>
            </span>
          </template>
        </DataTable>
      </Panel>

      <Panel title="Cloud" flush>
        <template #actions>
          <RouterLink :to="providerSettings" class="settings-link">Manage providers →</RouterLink>
        </template>
        <DataTable
          :columns="CLOUD_COLUMNS"
          :rows="cloud"
          :row-key="modelKey"
          caption="Cloud models"
          empty-title="No cloud models"
          empty-body="Connect a provider in Settings to route requests to hosted models."
        >
          <template #cell-name="{ row }">
            <span class="font-mono text-primary">{{ row.model_id || row.name }}</span>
          </template>
          <template #cell-routing="{ row }">
            <StatusTag v-if="routingRole(row)" :state="routingRole(row) === 'Primary' ? 'info' : 'neutral'" :label="routingRole(row)!" />
            <span v-else class="text-faint">{{ NO_VALUE }}</span>
          </template>
          <template #cell-credential="{ row }">
            <IdChip v-if="row.provider_config?.api_key_name" :id="row.provider_config.api_key_name" :visible="CREDENTIAL_CHARS" />
            <span v-else class="text-faint">{{ NO_VALUE }}</span>
          </template>
          <template #cell-actions="{ row }">
            <RouterLink :to="settingsFor(row)" class="settings-link">Settings</RouterLink>
          </template>
        </DataTable>
      </Panel>
    </template>
  </div>
</template>

<style scoped lang="postcss">
.settings-link {
  @apply rounded-[var(--radius-sm)] font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2;
}
</style>
