<script setup lang="ts">
import { ref, computed } from 'vue'
import { argsToString, stringToArgs } from '../../utils/config'
import { PROVIDER_STYLES } from '../../constants/providers'
import type { Model } from '../../types'

const props = defineProps<{
  model: Model
  state: import('../../types/admin').AdminState | null
  isEditing: boolean
}>()

const emit = defineEmits<{
  (e: 'start-model', name: string): void
  (e: 'stop-model'): void
  (e: 'remove-model', name: string): void
  (e: 'update-model', model: Model): void
  (e: 'cancel-edit'): void
  (e: 'start-edit', model: Model): void
}>()

const editingModel = ref<any>(null)

function initializeEdit() {
  editingModel.value = JSON.parse(JSON.stringify(props.model))
  if (!editingModel.value.provider_config) {
    editingModel.value.provider_config = { api_key_name: '' }
  }
}

const editingArgsStr = computed({
  get: () => argsToString(editingModel.value?.args),
  set: (val: string) => {
    if (editingModel.value) editingModel.value.args = stringToArgs(val)
  }
})

const availableKeys = computed(() => {
  if (!props.state || !editingModel.value) return []
  const cfg = props.state.config?.providers?.[editingModel.value.provider]
  return cfg?.api_keys || []
})

function saveEdit() {
  if (editingModel.value) {
    emit('update-model', editingModel.value)
  }
}
</script>

<template>
  <div :class="['model-item', model.active ? 'model-active' : 'model-inactive', isEditing ? 'model-editing' : '']">
    
    <!-- Normal View -->
    <div v-if="!isEditing" class="normal-view">
      <div class="content-left">
        <div class="model-name-group truncate" :title="model.name">
          <span :class="['provider-badge', PROVIDER_STYLES[model.provider as keyof typeof PROVIDER_STYLES]]">{{ model.provider }}</span>
          {{ model.name }}
          <span v-if="model.active && model.provider === 'local'" class="status-badge status-badge--online">Online</span>
          <span v-else-if="model.active" class="status-badge status-badge--selected">Selected</span>
        </div>
        <div class="model-meta truncate">
          <template v-if="model.provider === 'local'">
            Port: {{ model.port }} &bull; File: {{ model.filename }}
          </template>
          <template v-else>
            Model ID: {{ model.model_id || model.filename }}
          </template>
        </div>
        <div class="model-meta mt-1 truncate" v-if="model.args && model.args.length" :title="model.args.join(' ')">
          Args: <span class="args-text">{{ model.args.join(' ') }}</span>
        </div>
      </div>
      <div class="model-actions">
        <!-- Start/Stop only for local -->
        <template v-if="model.provider === 'local'">
          <button v-if="!model.active" @click="$emit('start-model', model.name)" class="btn-action-primary">Start</button>
          <button v-else @click="$emit('stop-model')" class="btn-action-stop">Stop</button>
        </template>
        <!-- Deactivate for cloud when active -->
        <template v-else-if="model.active">
          <button @click="$emit('stop-model')" class="btn-action-deselect">Deselect</button>
        </template>
        
        <button @click="initializeEdit(); $emit('start-edit', model)" class="btn-action-edit">Edit</button>
        <button 
          @click="$emit('remove-model', model.name)" 
          class="btn-action-remove"
          :class="{ 'btn-action-remove--disabled': model.active }"
          :disabled="model.active"
          title="Remove configuration"
        >
          Remove
        </button>
      </div>
    </div>

    <!-- Edit View -->
    <div v-else class="edit-view">
      <div class="edit-header">Edit {{ model.name }}</div>
      <div class="edit-grid">
        <template v-if="model.provider === 'local'">
          <div class="form-col-1-edit">
            <label class="form-label">Port</label>
            <input v-model.number="editingModel.port" type="number" class="form-input">
          </div>
          <div class="form-col-3-edit">
            <label class="form-label">Specific Args</label>
            <input v-model="editingArgsStr" type="text" placeholder="--ctx-size 8192" class="form-input font-mono">
          </div>
        </template>
        <template v-else>
          <div class="sm:col-span-2">
            <label class="form-label">Model ID</label>
            <input v-model="editingModel.model_id" type="text" class="form-input">
          </div>
          <div class="sm:col-span-2">
            <label class="form-label">API Key Name</label>
            <select v-model="editingModel.provider_config.api_key_name" class="form-input">
              <option value="">Default Key</option>
              <option v-for="k in availableKeys" :key="k.name" :value="k.name">{{ k.name }}</option>
            </select>
          </div>
        </template>
      </div>
      <div class="edit-actions">
        <button @click="$emit('cancel-edit')" class="btn-action-remove">Cancel</button>
        <button @click="saveEdit" class="btn-action-save">Save Changes</button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.model-item {
  @apply p-4 rounded-lg border transition-all;
}

.model-active {
  @apply bg-blue-900/10 border-blue-500/50 shadow-[0_0_15px_rgba(59,130,246,0.05)];
}

.model-inactive {
  @apply bg-gray-900/50 border-gray-700 hover:border-gray-600;
}

.model-editing {
  @apply bg-gray-800 border-blue-500 ring-2 ring-blue-600/20 translate-x-1;
}

.normal-view {
  @apply w-full flex justify-between items-center gap-4;
}

.content-left {
  @apply min-w-0 flex-1;
}

.model-name-group {
  @apply font-semibold text-white flex items-center gap-2;
}

.provider-badge {
  @apply px-1.5 py-0.5 rounded text-[9px] uppercase font-bold tracking-tight border;
}

.status-badge {
  @apply px-1.5 py-0.5 text-[9px] uppercase font-black rounded-full text-gray-950;
}

.status-badge--online {
  @apply bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.4)];
}

.status-badge--selected {
  @apply bg-purple-600 text-white shadow-[0_0_8px_rgba(147,51,234,0.4)];
}

.model-meta {
  @apply text-[11px] text-gray-500 mt-1;
}

.args-text {
  @apply font-mono text-[10px] text-gray-400;
}

.model-actions {
  @apply flex gap-2 items-center flex-wrap justify-end shrink-0;
}

.btn-action-primary {
  @apply px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-[11px] font-bold rounded shadow-lg transition-all active:scale-95;
}

.btn-action-stop {
  @apply px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-[11px] font-bold rounded shadow-lg transition-all active:scale-95;
}

.btn-action-deselect {
  @apply px-3 py-1.5 bg-gray-700 hover:bg-gray-600 text-white text-[11px] font-bold rounded shadow-lg transition-all active:scale-95;
}

.btn-action-edit {
  @apply text-[11px] px-2 py-1.5 text-gray-400 hover:text-white transition-colors;
}

.btn-action-remove {
  @apply px-2 py-1.5 text-[11px] text-gray-500 hover:text-red-400 transition-colors;
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
</style>
