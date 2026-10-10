<script setup lang="ts">
import { computed } from "vue";
import { formatBytes, formatParameters } from "../../utils/format/formatters";
import { formatTokenCount } from "../../utils/format/units";
import BaseButton from "../common/buttons/BaseButton.vue";
import Panel from "../common/layout/Panel.vue";
import DataTable from "../common/display/DataTable.vue";
import MicroLabel from "../common/display/MicroLabel.vue";
import StatusTag from "../common/display/StatusTag.vue";
import FormField from "../common/forms/FormField.vue";
import EmptyState from "../common/feedback/EmptyState.vue";
import ModelTuningFields from "./ModelTuningFields.vue";
import type { APIKeyItem, ProviderType } from "../../types/admin";
import type { Model, AvailableModel } from "../../types/model";
import type { DataTableColumn } from "../../types/ui";
import { useProviderModels } from "../../composables/settings/useProviderModels";

// A provider's configured models (grouped by the API key they use) and, for
// the local engine, the GGUF files found on disk. Adding, editing and removing
// apply immediately through the model registry; removals are confirmed by
// useProviderModels.
const props = defineProps<{
  provider: ProviderType;
  apiKeys: APIKeyItem[];
  models: Model[];
  availableModels?: AvailableModel[];
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
}>();

const NO_VALUE = "—";
const LOCAL = "local";

const {
  providerModels,
  isLoadingModels,
  editingModel,
  isAddingNew,
  modelForm,
  filterText,
  filteredProviderModels,
  groupsByKey,
  alreadyConfiguredFilenames,
  editingArgsStr,
  isSubmitDisabled,
  agentDefaults,
  addFormWorkload,
  loadModels,
  startAdd,
  scanAndAdd,
  cancelEdit,
  saveNewModel,
  addDiscoveredModel,
  handleClearAll,
  handleEdit,
  saveEdit,
  handleRemove,
  loopStrategyOptions,
} = useProviderModels(props, emit);

const isLocal = computed(() => props.provider === LOCAL);
const formTitle = computed(() => (isAddingNew.value ? "Add model" : `Edit ${editingModel.value?.name ?? "model"}`));

const modelKey = (m: Model) => m.name;
const MODEL_COLUMNS = computed<DataTableColumn<Model>[]>(() => [
  { key: "name", label: "Name", value: (m) => m.name },
  { key: "id", label: isLocal.value ? "File" : "Model ID" },
  ...(isLocal.value ? [{ key: "port", label: "Port", numeric: true, value: (m: Model) => (m.port ? `:${m.port}` : NO_VALUE) }] : []),
  { key: "actions", label: "Actions" },
]);

const fileKey = (m: AvailableModel) => m.filename;
const DISCOVERED_COLUMNS: DataTableColumn<AvailableModel>[] = [
  { key: "name", label: "Model" },
  { key: "file", label: "File" },
  { key: "size", label: "Size", numeric: true, value: (m) => formatBytes(m.size_bytes) },
  { key: "params", label: "Params", numeric: true, value: (m) => (m.metadata?.parameters ? formatParameters(m.metadata.parameters) : NO_VALUE) },
  { key: "context", label: "Context", numeric: true, value: (m) => (m.metadata?.context_length ? formatTokenCount(m.metadata.context_length) : NO_VALUE) },
  { key: "actions", label: "Actions" },
];
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel v-if="editingModel" :title="formTitle" preserve-case>
      <form class="flex flex-col gap-5" @submit.prevent="!isSubmitDisabled && (isAddingNew ? saveNewModel() : saveEdit())">
        <div class="field-grid">
          <!-- Add a local model -->
          <template v-if="isAddingNew && isLocal">
            <FormField label="Model filename" hint="A .gguf file in the model directory.">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="modelForm.filename" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="qwen2.5-7b-instruct-q4_k_m.gguf" autocomplete="off" />
              </template>
            </FormField>
            <FormField label="Friendly name" hint="Optional. Derived from the filename when empty.">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="modelForm.name" :aria-describedby="describedBy" type="text" class="form-control" autocomplete="off" />
              </template>
            </FormField>
            <FormField label="Port">
              <template #default="{ id }">
                <input :id="id" v-model.number="modelForm.port" type="number" class="form-control font-mono tabular-nums" />
              </template>
            </FormField>
            <FormField label="Custom arguments" hint="Added to the default arguments for this model.">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="modelForm.args" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="--ctx-size 8192 --gpu-layers 32" spellcheck="false" />
              </template>
            </FormField>
          </template>

          <!-- Add a cloud model -->
          <template v-else-if="isAddingNew">
            <FormField label="API key" hint="The credential this model is called with.">
              <template #default="{ id, describedBy }">
                <select :id="id" v-model="modelForm.key" :aria-describedby="describedBy" class="form-control">
                  <option value="">Choose a key</option>
                  <option v-for="key in apiKeys" :key="key.id" :value="key.name">{{ key.name }}</option>
                </select>
              </template>
            </FormField>
            <FormField label="Model ID" :hint="providerModels.length ? `${filteredProviderModels.length} of ${providerModels.length} models` : 'Type an ID, or scan the endpoint for its models.'">
              <template #default="{ id, describedBy }">
                <div v-if="providerModels.length" class="flex flex-col gap-1.5">
                  <div class="flex items-center gap-2">
                    <input v-model="filterText" type="search" aria-label="Filter models" class="form-control flex-1" placeholder="Filter models…" autocomplete="off" />
                    <BaseButton variant="ghost" size="sm" icon="refresh" icon-only label="Reload the model list" :disabled="!modelForm.key" :loading="isLoadingModels" @click="loadModels(modelForm.key)" />
                  </div>
                  <select :id="id" v-model="modelForm.id" :aria-describedby="describedBy" size="8" class="form-control font-mono">
                    <option v-for="m in filteredProviderModels" :key="m.id" :value="m.id">{{ m.id }}</option>
                  </select>
                  <p v-if="!filteredProviderModels.length && filterText" class="m-0 text-[length:var(--text-small)] text-muted">No model matches “{{ filterText }}”.</p>
                </div>
                <div v-else class="flex items-center gap-2">
                  <input :id="id" v-model="modelForm.id" :aria-describedby="describedBy" type="text" class="form-control flex-1 font-mono" placeholder="gpt-4o" autocomplete="off" />
                  <BaseButton variant="secondary" size="sm" icon="search" :disabled="!modelForm.key" :loading="isLoadingModels" @click="loadModels(modelForm.key)">Scan endpoint</BaseButton>
                </div>
              </template>
            </FormField>
            <FormField label="Friendly name" hint="Optional. Derived from the model ID when empty.">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="modelForm.name" :aria-describedby="describedBy" type="text" class="form-control" autocomplete="off" />
              </template>
            </FormField>
          </template>

          <!-- Edit a local model -->
          <template v-else-if="isLocal">
            <FormField label="Name">
              <template #default="{ id }">
                <input :id="id" v-model="editingModel.name" type="text" class="form-control" autocomplete="off" />
              </template>
            </FormField>
            <FormField label="Filename">
              <template #default="{ id }">
                <input :id="id" v-model="editingModel.filename" type="text" class="form-control font-mono" autocomplete="off" />
              </template>
            </FormField>
            <FormField label="Port">
              <template #default="{ id }">
                <input :id="id" v-model.number="editingModel.port" type="number" class="form-control font-mono tabular-nums" />
              </template>
            </FormField>
            <FormField label="Custom arguments" hint="Added to the default arguments for this model.">
              <template #default="{ id, describedBy }">
                <input :id="id" v-model="editingArgsStr" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="--ctx-size 8192 --gpu-layers 32" spellcheck="false" />
              </template>
            </FormField>
          </template>

          <!-- Edit a cloud model -->
          <template v-else>
            <FormField label="Name">
              <template #default="{ id }">
                <input :id="id" v-model="editingModel.name" type="text" class="form-control" autocomplete="off" />
              </template>
            </FormField>
            <FormField label="Model ID">
              <template #default="{ id }">
                <input :id="id" v-model="editingModel.model_id" type="text" class="form-control font-mono" autocomplete="off" />
              </template>
            </FormField>
            <FormField v-if="editingModel.provider_config" label="API key">
              <template #default="{ id }">
                <select :id="id" v-model="editingModel.provider_config.api_key_name" class="form-control">
                  <option value="">No key (use default)</option>
                  <option v-for="key in apiKeys" :key="key.id" :value="key.name">{{ key.name }}</option>
                </select>
              </template>
            </FormField>
          </template>
        </div>

        <ModelTuningFields
          v-if="isAddingNew"
          :model="modelForm"
          :provider="provider"
          :workload-class="addFormWorkload"
          :reasoning="agentDefaults?.reasoning"
          :loop-strategy-options="loopStrategyOptions"
        />
        <ModelTuningFields
          v-else
          :model="editingModel"
          :provider="provider"
          :workload-class="editingModel.workload_class || (isLocal ? 'local' : 'cloud')"
          :reasoning="agentDefaults?.reasoning"
          :loop-strategy-options="loopStrategyOptions"
        />

        <div class="flex flex-wrap justify-end gap-2">
          <BaseButton variant="ghost" @click="cancelEdit">Cancel</BaseButton>
          <BaseButton type="submit" variant="primary" icon="check" :disabled="isSubmitDisabled">{{ isAddingNew ? "Add model" : "Save model" }}</BaseButton>
        </div>
      </form>
    </Panel>

    <template v-else>
      <Panel v-if="isLocal && availableModels?.length" title="Discovered on disk" flush>
        <template #actions>
          <span class="font-mono text-[length:var(--text-small)] tabular-nums text-muted">{{ availableModels.length }} files</span>
          <BaseButton variant="ghost" size="sm" icon="refresh" @click="emit('refresh')">Rescan</BaseButton>
        </template>
        <DataTable :columns="DISCOVERED_COLUMNS" :rows="availableModels" :row-key="fileKey" caption="GGUF files on disk">
          <template #cell-name="{ row }">
            <span class="text-primary">{{ row.metadata?.name || row.name }}</span>
            <span v-if="row.metadata?.architecture || row.metadata?.quantization" class="ml-2 font-mono text-[length:var(--text-micro)] uppercase text-faint">
              {{ [row.metadata?.architecture, row.metadata?.quantization].filter(Boolean).join(" · ") }}
            </span>
          </template>
          <template #cell-file="{ row }">
            <span class="block max-w-[220px] truncate font-mono text-[length:var(--text-small)] text-muted" :title="row.filename">{{ row.filename }}</span>
          </template>
          <template #cell-actions="{ row }">
            <StatusTag v-if="alreadyConfiguredFilenames.has(row.filename)" state="neutral" label="Added" />
            <BaseButton v-else variant="ghost" size="sm" icon="plus" icon-only :label="`Add ${row.metadata?.name || row.name}`" @click="addDiscoveredModel(row)" />
          </template>
        </DataTable>
      </Panel>

      <Panel :title="isLocal ? 'Local models' : 'Models'">
        <template #actions>
          <BaseButton v-if="models.length" variant="ghost" size="sm" icon="trash" @click="handleClearAll">Remove all</BaseButton>
          <BaseButton variant="primary" size="sm" icon="plus" @click="startAdd">Add model</BaseButton>
        </template>

        <EmptyState
          v-if="!groupsByKey.length"
          :title="isLocal ? 'No local models' : 'No models yet'"
          :body="isLocal ? 'Add a GGUF file from the model directory to serve it through llama.cpp.' : 'Add an API key above, then add a model it can call.'"
        />

        <div v-else class="flex flex-col gap-5">
          <section v-for="group in groupsByKey" :key="group.keyName || 'no-key'" class="flex flex-col gap-2">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="flex items-center gap-3">
                <MicroLabel>{{ group.keyName || "No key assigned" }}</MicroLabel>
                <span class="font-mono text-[length:var(--text-micro)] tabular-nums text-faint">{{ group.models.length }}</span>
              </span>
              <BaseButton v-if="group.keyName && !isLocal" variant="ghost" size="sm" icon="search" @click="scanAndAdd(group.keyName)">Discover models</BaseButton>
            </div>
            <DataTable
              :columns="MODEL_COLUMNS"
              :rows="group.models"
              :row-key="modelKey"
              :caption="group.keyName ? `Models using ${group.keyName}` : 'Models without a key'"
              empty-title="No models for this key yet"
              empty-body="Discover the endpoint's models, or add one by ID."
            >
              <template #cell-id="{ row }">
                <span class="font-mono text-[length:var(--text-small)] text-muted">{{ row.model_id || row.filename || NO_VALUE }}</span>
              </template>
              <template #cell-actions="{ row }">
                <span class="inline-flex items-center gap-1">
                  <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${row.name}`" @click="handleEdit(row)" />
                  <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Remove ${row.name}`" @click="handleRemove(row.name)" />
                </span>
              </template>
            </DataTable>
          </section>
        </div>
      </Panel>
    </template>
  </div>
</template>

<style scoped lang="postcss">
.field-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4;
}
</style>
