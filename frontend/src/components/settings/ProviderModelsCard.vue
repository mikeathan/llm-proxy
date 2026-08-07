<script setup lang="ts">
import { formatBytes, formatParameters } from "../../utils/format/formatters";
import BaseButton from "../common/buttons/BaseButton.vue";
import Icon from "../icons/Icon.vue";
import ModelTuningFields from "./ModelTuningFields.vue";
import { PROVIDER_STYLES } from "../../constants/providers";
import type { APIKeyItem, ProviderType } from "../../types/admin";
import type { Model, AvailableModel } from "../../types/model";
import { useProviderModels } from "../../composables/settings/useProviderModels";

const props = defineProps<{
  provider: ProviderType;
  apiKeys: APIKeyItem[];
  models: Model[];
  availableModels?: AvailableModel[];
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
}>();

const {
  providerModels,
  isLoadingModels,
  editingModel,
  isAddingNew,
  modelForm,
  filterText,
  filteredProviderModels,
  groupsByKey,
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
} = useProviderModels(props, emit);
</script>

<template>
  <div class="provider-models-card">
    <div class="card-header">
      <h3 class="card-title">Models</h3>
      <div class="header-actions">
        <BaseButton
          v-if="!editingModel && props.models.length > 0"
          variant="secondary"
          size="sm"
          icon="trash"
          className="mr-2 text-red-400 border-red-400/30 hover:bg-red-500/10"
          @click="handleClearAll"
        >
          Clear All
        </BaseButton>
        <BaseButton
          v-if="!editingModel"
          variant="primary"
          size="sm"
          icon="plus"
          @click="startAdd"
        >
          Add Model
        </BaseButton>
      </div>
    </div>

    <!-- Add/Edit Form -->
    <div v-if="editingModel" class="form-panel">
      <h4 class="form-title">
        {{ isAddingNew ? "Add Model" : "Edit Model" }}
      </h4>
      <div class="form-body">
        <template v-if="isAddingNew">
          <template v-if="provider === 'local'">
            <div class="form-group">
              <label class="form-label">Model Filename</label>
              <input
                v-model="modelForm.filename"
                type="text"
                class="form-input"
                placeholder="e.g. qwen2.5-7b-instruct-q4_k_m.gguf"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Friendly Name</label>
              <input
                v-model="modelForm.name"
                type="text"
                class="form-input"
                placeholder="Auto-derived from filename if empty"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Port</label>
              <input
                v-model.number="modelForm.port"
                type="number"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Custom Arguments</label>
              <input
                v-model="modelForm.args"
                type="text"
                class="form-input"
                placeholder="--ctx-size 8192 --gpu-layers 32"
              />
            </div>
            <ModelTuningFields :model="modelForm" provider="local" workload-class="local" :reasoning="agentDefaults?.reasoning" />
          </template>
          <template v-else>
            <div class="form-group">
              <label class="form-label">API Key</label>
              <select v-model="modelForm.key" class="form-input">
                <option value="">No key (use default)</option>
                <option
                  v-for="key in apiKeys"
                  :key="key.id"
                  :value="key.name"
                >
                  {{ key.name }}
                </option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">Model ID</label>
              <div v-if="providerModels.length > 0" class="model-select-area">
                <input
                  v-model="filterText"
                  type="text"
                  class="form-input model-filter-input"
                  placeholder="Type to filter models..."
                />
                <select
                  v-model="modelForm.id"
                  class="form-input model-filter-select"
                  size="5"
                >
                  <option
                    v-for="m in filteredProviderModels"
                    :key="m.id"
                    :value="m.id"
                  >
                    {{ m.id }}
                  </option>
                </select>
                <div v-if="filteredProviderModels.length === 0 && filterText" class="filter-no-results">
                  No models match "{{ filterText }}"
                </div>
                <div class="model-select-toolbar">
                  <span class="model-count-hint">
                    {{ filteredProviderModels.length }} of {{ providerModels.length }} models
                  </span>
                  <BaseButton
                    variant="ghost"
                    size="sm"
                    icon="spinner"
                    iconOnly
                    :loading="isLoadingModels"
                    @click="loadModels"
                    title="Refresh model list"
                  />
                </div>
              </div>
                <div v-else class="flex flex-col gap-2">
                  <input
                    v-model="modelForm.id"
                    type="text"
                    class="form-input"
                    placeholder="e.g. gpt-4o"
                  />
                  <BaseButton
                    variant="secondary"
                    size="sm"
                    icon="spinner"
                    :loading="isLoadingModels"
                    @click="loadModels(modelForm.key)"
                    class="w-fit"
                  >
                    Scan Endpoint for Models
                  </BaseButton>
                </div>
              </div>
            <div class="form-group">
              <label class="form-label">Friendly Name</label>
              <input
                v-model="modelForm.name"
                type="text"
                class="form-input"
                placeholder="Auto-derived from model ID"
              />
            </div>
            <ModelTuningFields :model="modelForm" :provider="provider" :workload-class="addFormWorkload" :reasoning="agentDefaults?.reasoning" />
          </template>
        </template>
        <template v-else>
          <template v-if="provider === 'local'">
            <div class="form-group">
              <label class="form-label">Name</label>
              <input
                v-model="editingModel.name"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Filename</label>
              <input
                v-model="editingModel.filename"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Port</label>
              <input
                v-model.number="editingModel.port"
                type="number"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Custom Arguments</label>
              <input
                v-model="editingArgsStr"
                type="text"
                class="form-input"
                placeholder="--ctx-size 8192 --gpu-layers 32"
              />
            </div>
            <ModelTuningFields :model="editingModel" provider="local" :workload-class="editingModel.workload_class || 'local'" :reasoning="agentDefaults?.reasoning" />
          </template>
          <template v-else>
            <div class="form-group">
              <label class="form-label">Name</label>
              <input
                v-model="editingModel.name"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <div class="flex items-center justify-between mb-1">
                <label class="form-label mb-0">Model ID</label>
              </div>
              <input
                v-model="editingModel.model_id"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">API Key</label>
              <select
                v-model="editingModel.provider_config!.api_key_name"
                class="form-input"
              >
                <option value="">No key (use default)</option>
                <option
                  v-for="key in apiKeys"
                  :key="key.id"
                  :value="key.name"
                >
                  {{ key.name }}
                </option>
              </select>
            </div>
            <ModelTuningFields :model="editingModel" :provider="provider" :workload-class="editingModel.workload_class || 'cloud'" :reasoning="agentDefaults?.reasoning" />
          </template>
        </template>
      </div>
      <div class="form-actions">
        <BaseButton variant="secondary" size="sm" @click="cancelEdit">
          Cancel
        </BaseButton>
        <BaseButton
          variant="primary"
          size="sm"
          icon="check"
          :disabled="isSubmitDisabled"
          @click="isAddingNew ? saveNewModel() : saveEdit()"
        >
          {{ isAddingNew ? "Add" : "Save" }}
        </BaseButton>
      </div>
    </div>

    <!-- Model list -->
    <div v-if="!editingModel" class="models-list">


      <!-- Discovered GGUF (local only) -->
      <div
        v-if="provider === 'local' && availableModels && availableModels.length > 0"
        class="discovered-section"
      >
        <div class="discovered-header">
          <div class="flex items-center gap-2">
            <span class="discovered-title">Discovered on Disk</span>
            <button 
              class="refresh-btn" 
              @click="emit('refresh')"
              title="Rescan model directory"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>
            </button>
          </div>
          <span class="discovered-count">{{ availableModels.length }} models</span>
        </div>
        <div class="discovered-list">
          <div
            v-for="m in availableModels"
            :key="m.filename"
            class="discovered-row"
          >
            <div class="discovered-info">
              <div class="discovered-primary">
                <span class="discovered-name">{{ m.metadata?.name || m.name }}</span>
                <div v-if="m.metadata" class="meta-pills">
                  <span v-if="m.metadata.architecture" class="pill pill--blue">{{ m.metadata.architecture }}</span>
                  <span v-if="m.metadata.quantization" class="pill pill--green">{{ m.metadata.quantization }}</span>
                </div>
              </div>
              <div class="discovered-secondary">
                <span class="discovered-filename">{{ m.filename }}</span>
                <span class="discovered-detail-dot">•</span>
                <span class="discovered-detail-text">{{ formatBytes(m.size_bytes) }}</span>
                <template v-if="m.metadata">
                  <span v-if="m.metadata.parameters" class="discovered-detail-dot">•</span>
                  <span v-if="m.metadata.parameters" class="discovered-detail-text">{{ formatParameters(m.metadata.parameters) }} params</span>
                  <span v-if="m.metadata.context_length" class="discovered-detail-dot">•</span>
                  <span v-if="m.metadata.context_length" class="discovered-detail-text">{{ m.metadata.context_length }} ctx</span>
                </template>
              </div>
            </div>
            <BaseButton
              variant="secondary"
              size="sm"
              @click="addDiscoveredModel(m)"
            >
              Add
            </BaseButton>
          </div>
        </div>
      </div>



      <div
        v-for="group in groupsByKey"
        :key="group.keyName || 'no-key'"
        class="model-group"
      >
        <div class="group-header">
          <div class="flex items-center gap-3">
            <span class="group-key-label">
              {{ group.keyName || "No key assigned" }}
            </span>
            <span class="group-count">{{ group.models.length }} model(s)</span>
          </div>
          <BaseButton
            v-if="group.keyName"
            variant="ghost"
            size="sm"
            icon="search"
            @click="scanAndAdd(group.keyName)"
            title="Scan this endpoint and add a model"
          >
            Discover Models
          </BaseButton>
        </div>
        
        <div v-if="group.models.length === 0" class="py-3 px-4 text-xs text-gray-500 italic border border-dashed border-gray-700/50 rounded-lg mx-2 my-2 bg-gray-800/30">
          No models configured for this endpoint yet.
        </div>

        <div
          v-for="m in group.models"
          :key="m.name"
          class="model-row"
        >
          <div class="model-info">
            <span
              :class="[
                'provider-badge',
                PROVIDER_STYLES[m.provider as keyof typeof PROVIDER_STYLES],
              ]"
            >
              {{ m.provider }}
            </span>
            <span class="model-name">{{ m.name }}</span>
            <span class="model-id">{{ m.model_id || m.filename }}</span>
          </div>
          <div class="model-actions">
            <button
              @click="handleEdit(m)"
              class="action-btn"
              title="Edit"
            >
              <Icon name="edit" size="sm" />
            </button>
            <button
              @click="handleRemove(m.name)"
              class="action-btn action-btn-remove"
              title="Remove"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.provider-models-card {
  @apply bg-gray-800/50 border border-gray-700 rounded-lg p-4 space-y-3;
}

.card-header {
  @apply flex items-center justify-between;
}

.card-title {
  @apply text-sm font-bold text-white;
}

.form-panel {
  @apply bg-gray-900/60 border border-blue-600/30 rounded-lg p-4 space-y-3;
}

.form-title {
  @apply text-xs font-bold text-blue-400 uppercase tracking-wide;
}

.form-body {
  @apply space-y-3;
}

.form-group {
  @apply space-y-1;
}

.form-label {
  @apply block text-xs font-semibold text-gray-400;
}

.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white
         focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}

select.form-input {
  @apply appearance-none bg-no-repeat pr-10
         bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236b7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22m6%208%204%204%204-4%22%2F%3E%3C%2Fsvg%3E')]
         bg-[position:right_0.5rem_center] bg-[length:1.25rem_1.25rem];
}

.model-select-area {
  @apply space-y-1;
}

.model-filter-input {
  @apply text-xs;
}

.model-filter-select {
  @apply h-auto min-h-[100px] max-h-[200px] appearance-none bg-gray-900 border-gray-700
         overflow-y-auto text-xs font-mono pr-3;
}

.model-filter-select option {
  @apply px-2 py-1;
}

.model-filter-select option:checked {
  @apply bg-blue-600/30;
}

.filter-no-results {
  @apply text-xs text-gray-500 italic px-1;
}

.model-select-toolbar {
  @apply flex items-center justify-between;
}

.model-count-hint {
  @apply text-[10px] text-gray-600;
}

.option-muted {
  @apply opacity-50;
}

.form-actions {
  @apply flex justify-end gap-2 pt-2 border-t border-gray-700/50;
}

.models-list {
  @apply space-y-3;
}

.models-empty {
  @apply text-center text-xs text-gray-500 italic py-6;
}

.link-btn {
  @apply text-blue-400 hover:text-blue-300 underline;
}

.model-group {
  @apply space-y-1;
}

.group-header {
  @apply flex items-center justify-between px-2 py-1;
}

.group-key-label {
  @apply text-xs font-bold text-gray-400;
}

.group-count {
  @apply text-[10px] text-gray-600;
}

.model-row {
  @apply flex items-center justify-between px-3 py-2 rounded-md
         bg-gray-900/40 border border-gray-700/50 hover:border-gray-600 transition-all;
}

.model-info {
  @apply flex items-center gap-2 min-w-0 flex-1;
}

.provider-badge {
  @apply px-1.5 py-0.5 rounded text-[8px] uppercase font-black tracking-widest border shrink-0;
}

.model-name {
  @apply text-sm font-bold text-white truncate;
}

.model-id {
  @apply text-[10px] text-gray-500 font-mono truncate;
}

.model-actions {
  @apply flex gap-1 shrink-0;
}

.action-btn {
  @apply p-1.5 rounded text-gray-600 hover:text-white hover:bg-gray-800/50 transition-all;
}

.action-btn-remove:hover {
  @apply text-red-500 bg-red-500/10;
}

.form-section-divider {
  @apply text-xs font-bold text-blue-400 uppercase tracking-wide border-t border-gray-700 pt-3 mt-2;
}

.tuning-grid {
  @apply grid grid-cols-1 sm:grid-cols-2 gap-3;
}

.discovered-section {
  @apply bg-gray-900/40 border border-gray-700/50 rounded-lg overflow-hidden;
}

.discovered-header {
  @apply flex items-center justify-between px-4 py-2 bg-gray-800/50 border-b border-gray-700/50;
}

.discovered-title {
  @apply text-[10px] font-bold text-gray-400 uppercase tracking-widest;
}

.discovered-count {
  @apply text-[10px] text-gray-600 font-mono;
}

.discovered-list {
  @apply divide-y divide-gray-700/30;
}

.discovered-row {
  @apply flex items-center justify-between px-4 py-3
         hover:bg-gray-800/20 transition-all;
}

.discovered-info {
  @apply flex-1 min-w-0 flex flex-col gap-1;
}

.discovered-primary {
  @apply flex items-center gap-3;
}

.discovered-name {
  @apply text-[13px] font-bold text-white truncate;
}

.discovered-secondary {
  @apply flex items-center gap-2 min-w-0;
}

.discovered-filename {
  @apply text-[10px] text-gray-500 font-mono truncate max-w-[180px];
}

.discovered-detail-text {
  @apply text-[10px] text-gray-500 font-medium whitespace-nowrap;
}

.discovered-detail-dot {
  @apply text-gray-700 text-[10px] select-none;
}

.meta-pills {
  @apply flex items-center gap-1.5 flex-wrap;
}

.pill {
  @apply text-[9px] font-black uppercase tracking-wider px-1.5 py-0.5 rounded-[4px] border leading-none;
}

.pill--blue {
  @apply bg-blue-500/10 text-blue-400 border-blue-500/20;
}

.pill--green {
  @apply bg-emerald-500/10 text-emerald-400 border-emerald-500/20;
}

.pill--gray {
  @apply bg-gray-700/30 text-gray-400 border-gray-600/20;
}

.pill--purple {
  @apply bg-indigo-500/10 text-indigo-400 border-indigo-500/20;
}

.discovered-row {
  @apply flex items-center justify-between px-4 py-3
         hover:bg-gray-800/20 transition-all border-b border-gray-700/30 last:border-0;
}

.discovered-header {
  @apply flex items-center justify-between px-4 py-2.5 bg-gray-800/40 border-b border-gray-700/50;
}

.discovered-title {
  @apply text-[10px] font-bold text-gray-400 uppercase tracking-widest;
}

.discovered-count {
  @apply text-[10px] text-gray-600 font-mono;
}

.refresh-btn {
  @apply p-1 rounded-full text-gray-600 hover:text-blue-400 hover:bg-blue-400/10 transition-all;
}

.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
