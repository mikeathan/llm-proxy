<script setup lang="ts">
import { ref, computed } from "vue";
import { argsToString, stringToArgs } from "../../utils/config";
import { PROVIDER_STYLES } from "../../constants/providers";
import type { Model, AdminState } from "../../types";
import ModelTags from "./ModelTags.vue";
import { inferMetadata } from "../../utils/model-discovery";

const props = defineProps<{
  model: Model;
  state: AdminState | null;
  isEditing: boolean;
}>();

// use actual metadata if present, otherwise infer from name/id
const displayMetadata = computed(() => {
  if (props.model.metadata && Object.keys(props.model.metadata).length > 0) {
    return props.model.metadata;
  }
  return inferMetadata(props.model.model_id || props.model.name);
});

const emit = defineEmits<{
  (e: "start-model", model: Model): void;
  (e: "stop-model", model: Model): void;
  (e: "deselect-model", model: Model): void;
  (e: "select-model", model: Model): void;
  (e: "remove-model", name: string): void;
  (e: "update-model", model: Model): void;
  (e: "cancel-edit"): void;
  (e: "start-edit", model: Model): void;
}>();

const editingModel = ref<any>(null);

function initializeEdit() {
  editingModel.value = JSON.parse(JSON.stringify(props.model));
  if (!editingModel.value.provider_config) {
    editingModel.value.provider_config = { api_key_name: "" };
  }
}

function startEdit() {
  initializeEdit();
  emit("start-edit", props.model);
}

const editingArgsStr = computed({
  get: () => argsToString(editingModel.value?.args),
  set: (val: string) => {
    if (editingModel.value) editingModel.value.args = stringToArgs(val);
  },
});

const availableKeys = computed(() => {
  if (!props.state || !editingModel.value) return [];
  const cfg = props.state.config?.providers?.[editingModel.value.provider];
  return cfg?.api_keys || [];
});

function saveEdit() {
  if (editingModel.value) {
    emit("update-model", editingModel.value);
  }
}
</script>

<template>
  <div
    :class="[
      'model-item',
      model.active ? 'model-active' : 'model-inactive',
      isEditing ? 'model-editing' : '',
    ]"
  >
    <!-- Normal View (Ultra-Compact & Professional) -->
    <div v-if="!isEditing" class="normal-view">
      <div class="content-main">
        <!-- Identity & Primary Actions -->
        <div class="identity-row">
          <div class="model-identity min-w-0 flex-1">
            <span :class="['provider-badge', PROVIDER_STYLES[model.provider as keyof typeof PROVIDER_STYLES]]">{{ model.provider }}</span>
            <span class="model-name-text truncate">{{ model.name }}</span>
          </div>

          <!-- Status Dot & Inline Tags (Desktop) -->
          <div class="identity-meta hidden sm:flex items-center gap-3 ml-2">
            <div class="status-indicator" v-if="model.active">
              <span v-if="model.provider === 'local'" class="status-dot status-dot--online" title="Online"></span>
              <span v-else class="status-dot status-dot--selected" title="Selected"></span>
            </div>
            <div class="tags-inline" v-if="displayMetadata">
              <ModelTags :metadata="displayMetadata" />
            </div>
          </div>

          <!-- Compact Action Bar -->
          <div class="action-bar ml-auto">
            <!-- Start/Stop/Select/Deselect Toggle -->
            <button 
              v-if="model.provider === 'local'"
              @click="model.active ? $emit('stop-model', model) : $emit('start-model', model)"
              :class="['btn-icon-toggle', model.active ? 'btn-icon-stop' : 'btn-icon-start']"
              :title="model.active ? 'Stop Model' : 'Start Model'"
            >
              <svg v-if="!model.active" viewBox="0 0 24 24" fill="currentColor" class="w-4 h-4"><path d="M8 5v14l11-7z"/></svg>
              <svg v-else viewBox="0 0 24 24" fill="currentColor" class="w-4 h-4"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>
            </button>

            <button 
              v-else
              @click="model.active ? $emit('deselect-model', model) : $emit('select-model', model)"
              :class="['btn-icon-toggle', model.active ? 'btn-icon-stop' : 'btn-icon-start']"
              :title="model.active ? 'Deselect' : 'Select'"
            >
              <svg v-if="!model.active" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" class="w-4 h-4"><path d="M5 13l4 4L19 7"/></svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" class="w-4 h-4"><path d="M18 6L6 18M6 6l12 12"/></svg>
            </button>

            <button @click="startEdit" class="btn-icon-utility" title="Edit Configuration">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="w-3.5 h-3.5"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </button>

            <button 
              @click="$emit('remove-model', model.name)" 
              class="btn-icon-utility btn-icon-remove"
              :disabled="model.active"
              title="Remove"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="w-3.5 h-3.5"><path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
            </button>
          </div>
        </div>

        <!-- Metadata & Mobile Tags -->
        <div class="details-row">
          <div class="info-line truncate">
            <span class="info-item" v-if="model.provider === 'local'">
              <span class="info-label">PORT</span> {{ model.port || 'Auto' }}
            </span>
            <span class="info-separator" v-if="model.provider === 'local'">&bull;</span>
            <span class="info-item truncate">
              <span class="info-label">{{ model.provider === 'local' ? 'FILE' : 'ID' }}</span> {{ model.filename }}
            </span>
          </div>

          <!-- Mobile Tags (Inline but scaled) -->
          <div class="tags-inline flex sm:hidden mt-1.5" v-if="displayMetadata">
            <ModelTags :metadata="displayMetadata" />
          </div>
        </div>
      </div>
    </div>

    <!-- Edit View -->
    <div v-else class="edit-view">
      <div class="edit-header">Edit {{ model.name }}</div>
      <div class="edit-grid">
        <template v-if="model.provider === 'local'">
          <div class="form-col-1-edit">
            <label class="form-label">Port</label>
            <input
              v-model.number="editingModel.port"
              type="number"
              class="form-input"
            />
          </div>
          <div class="form-col-3-edit">
            <label class="form-label">Specific Args</label>
            <input
              v-model="editingArgsStr"
              type="text"
              placeholder="--ctx-size 8192"
              class="form-input font-mono"
            />
          </div>
        </template>
        <template v-else>
          <div class="sm:col-span-2">
            <label class="form-label">Model ID</label>
            <input
              v-model="editingModel.model_id"
              type="text"
              class="form-input"
            />
          </div>
          <div class="sm:col-span-2">
            <label class="form-label">API Key Name</label>
            <select
              v-model="editingModel.provider_config.api_key_name"
              class="form-input"
            >
              <option value="">Default Key</option>
              <option v-for="k in availableKeys" :key="k.name" :value="k.name">
                {{ k.name }}
              </option>
            </select>
          </div>
        </template>
      </div>
      <div class="prefill-edit-row">
        <label class="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            :checked="editingModel.prefill"
            @change="editingModel.prefill = ($event.target as HTMLInputElement).checked"
            class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
          />
          <span class="text-sm text-gray-300">Prefill tool calls</span>
        </label>
        <p class="text-[10px] text-gray-500 mt-1 ml-6">
          Force the assistant response to start with a tool call in automation mode.
          Recommended for smaller local models that struggle with XML formatting.
        </p>
      </div>
      <div class="edit-actions">
        <button @click="$emit('cancel-edit')" class="btn-action-remove">
          Cancel
        </button>
        <button @click="saveEdit" class="btn-action-save">Save Changes</button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.model-item {
  @apply p-2.5 px-3.5 rounded-lg border transition-all duration-300 backdrop-blur-sm;
  background: linear-gradient(
    145deg,
    rgba(17, 24, 39, 0.4),
    rgba(17, 24, 39, 0.1)
  );
}

.model-active {
  @apply bg-blue-900/10 border-blue-500/40 shadow-[0_4px_20px_rgba(59,130,246,0.08)];
}

.model-inactive {
  @apply bg-gray-900/30 border-gray-800/50 hover:border-gray-700 hover:bg-gray-900/50;
}

.model-editing {
  @apply bg-gray-800 border-blue-500 ring-2 ring-blue-600/20 translate-x-1;
}

.normal-view {
  @apply w-full flex items-center gap-4;
}

.content-main {
  @apply min-w-0 flex-1 flex flex-col gap-0.5;
}

.identity-row {
  @apply flex items-center gap-2;
}

.model-identity {
  @apply flex items-center gap-2.5 font-bold text-white min-w-0;
}

.model-name-text {
  @apply text-[14px] tracking-tight;
}

.provider-badge {
  @apply px-1.5 py-0.5 rounded text-[8px] uppercase font-black tracking-widest border shrink-0 bg-gray-900/50;
}

.tags-inline {
  @apply items-center gap-1.5 overflow-hidden;
}

.details-row {
  @apply flex flex-col;
}

.info-line {
  @apply flex items-center gap-2 text-[10px] text-gray-500 font-medium;
}

.info-label {
  @apply text-[8px] uppercase font-black text-gray-700 tracking-tighter;
}

.info-separator {
  @apply text-gray-800;
}

.status-dot {
  @apply w-2 h-2 rounded-full shrink-0;
}

.status-dot--online {
  @apply bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.4)];
}

.status-dot--selected {
  @apply bg-purple-500 shadow-[0_0_8px_rgba(168,85,247,0.4)];
}

.action-bar {
  @apply flex items-center gap-1.5 shrink-0;
}

.btn-icon-toggle {
  @apply p-1.5 rounded-lg transition-all active:scale-90 flex items-center justify-center;
}

.btn-icon-start {
  @apply bg-blue-600/20 text-blue-400 hover:bg-blue-600 hover:text-white shadow-sm;
}

.btn-icon-stop {
  @apply bg-red-600/20 text-red-400 hover:bg-red-600 hover:text-white shadow-sm;
}

.btn-icon-utility {
  @apply p-1.5 rounded text-gray-600 hover:text-white hover:bg-gray-800/50 transition-all active:scale-90 flex items-center justify-center;
}

.btn-icon-remove:disabled {
  @apply opacity-10 cursor-not-allowed grayscale;
}

.btn-icon-remove:not(:disabled):hover {
  @apply text-red-500 bg-red-500/10;
}

.btn-action-remove--disabled {
  @apply opacity-20 cursor-not-allowed grayscale;
}

.edit-view {
  @apply w-full flex flex-col gap-3;
}

.edit-header {
  @apply font-medium text-white mb-1;
}

.edit-grid {
  @apply grid grid-cols-1 sm:grid-cols-4 gap-3;
}

.edit-actions {
  @apply flex justify-end gap-2 mt-2;
}

.btn-action-save {
  @apply px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-[11px] font-bold rounded shadow-lg transition-all active:scale-95;
}

.form-label {
  @apply block text-[10px] uppercase font-bold text-gray-500 mb-1.5 tracking-wider;
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

.form-col-1-edit {
  @apply sm:col-span-1;
}

.form-col-3-edit {
  @apply sm:col-span-3;
}

.prefill-edit-row {
  @apply mt-1;
}
</style>
