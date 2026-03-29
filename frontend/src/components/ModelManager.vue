<template>
  <div class="model-manager-container">
    <!-- Configured Models -->
    <div class="models-box">
      <h2 class="models-title">
        Configured Models
        <span class="models-count">{{ state?.models?.length || 0 }} Models</span>
      </h2>
      <div class="models-list-wrapper">
        <div v-if="!state?.models || state.models.length === 0" class="models-empty">
          No models configured. Select an available model below to add one.
        </div>
        <div class="models-list" v-else>
          <div v-for="model in state.models" :key="model.name"
               :class="['model-item', model.active ? 'model-active' : 'model-inactive', editingModel?.name === model.name ? 'model-editing' : '']">
            <!-- Normal View -->
            <div v-if="editingModel?.name !== model.name" class="w-full flex justify-between items-center gap-4">
              <div class="min-w-0 flex-1">
                <div class="model-name truncate" :title="model.name">
                  {{ model.name }}
                  <span v-if="model.active" class="model-badge">Active</span>
                </div>
                <div class="model-details truncate">Port: {{ model.port }} &bull; File: {{ model.filename }}</div>
                <div class="model-details mt-1 truncate" v-if="model.args && model.args.length" :title="model.args.join(' ')">
                  Args: <span class="font-mono text-[10px] text-gray-400">{{ model.args.join(' ') }}</span>
                </div>
              </div>
              <div class="model-actions shrink-0">
                <button v-if="!model.active" @click="$emit('startModel', model.name)" class="btn-start">Start</button>
                <button v-else @click="$emit('stopModel')" class="btn-stop-local">Stop</button>
                <button @click="startEdit(model)" class="btn-edit">Edit</button>
                <button @click="$emit('removeModel', model.name)" :class="['btn-remove', model.active ? 'btn-remove-disabled' : '']" :disabled="model.active">Remove</button>
              </div>
            </div>

            <!-- Edit View -->
            <div v-else class="w-full flex flex-col gap-3">
              <div class="font-medium text-white mb-1">Edit {{ model.name }}</div>
              <div class="grid grid-cols-4 gap-3">
                <div class="form-col-1-edit">
                  <label class="form-label">Port</label>
                  <input v-model.number="editingModel.port" type="number" class="form-input">
                </div>
                <div class="form-col-3-edit">
                  <label class="form-label">Specific Args (space separated)</label>
                  <input v-model="editingArgsStr" type="text" placeholder="--ctx-size 8192" class="form-input font-mono">
                </div>
              </div>
              <div class="flex justify-end gap-2 mt-2">
                <button @click="editingModel = null" class="btn-remove">Cancel</button>
                <button @click="saveEdit" class="btn-start !bg-blue-600 hover:!bg-blue-500">Save Changes</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Add / Discovered Models -->
    <div class="models-box">
      <h2 class="add-title">Add Model</h2>

      <form @submit.prevent="submitModel" class="add-form">
        <div class="form-grid-2">
          <div>
            <label class="form-label">Name</label>
            <input v-model="localNewModel.name" type="text" required placeholder="e.g. qwen2.5" class="form-input">
          </div>
          <div>
            <label class="form-label">Filename</label>
            <input v-model="localNewModel.filename" type="text" required placeholder="e.g. model.gguf" class="form-input">
          </div>
        </div>
        <div class="form-grid-3">
          <div class="form-col-1">
            <label class="form-label">Port</label>
            <input v-model.number="localNewModel.port" type="number" class="form-input">
          </div>
          <div class="form-col-2">
            <label class="form-label">Extra Args (space separated)</label>
            <input v-model="localNewModel.args" type="text" placeholder="-c 4096 --ngl 99" class="form-input font-mono">
          </div>
        </div>
        <button type="submit" class="btn-add">Add to Configuration</button>
      </form>

      <h3 class="discovered-title">Discovered in Directory</h3>
      <div class="discovered-wrapper">
        <div v-if="!availableModels || availableModels.length === 0" class="discovered-empty">
          No new .gguf files found in model directory.
        </div>
        <div class="discovered-list" v-else>
          <div v-for="model in availableModels" :key="model.filename" class="discovered-item group">
            <div class="discovered-details">
              <div class="discovered-name" :title="model.name">{{ model.name }}</div>
              <div class="discovered-file" :title="model.filename">{{ model.filename }}</div>
            </div>
            <button @click="selectAvailableModel(model)" class="btn-select group-hover:opacity-100 focus:opacity-100">Select</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { argsToString, stringToArgs } from '../utils/config'
import type { AdminState, AvailableModel, NewModelForm } from '../types'

const props = defineProps<{
  state: AdminState | null
  availableModels: AvailableModel[]
  newModel: NewModelForm
}>()

const emit = defineEmits<{
  (e: 'update:newModel', model: any): void
  (e: 'startModel', name: string): void
  (e: 'stopModel'): void
  (e: 'removeModel', name: string): void
  (e: 'updateModel', model: any): void
  (e: 'addModel'): void
}>()

const localNewModel = ref({ ...props.newModel })
const editingModel = ref<any>(null)

const editingArgsStr = computed({
  get: () => argsToString(editingModel.value?.args),
  set: (val: string) => {
    if (editingModel.value) editingModel.value.args = stringToArgs(val)
  }
})

function startEdit(model: any) {
  editingModel.value = JSON.parse(JSON.stringify(model))
}

function saveEdit() {
  if (editingModel.value) {
    emit('updateModel', editingModel.value)
    editingModel.value = null
  }
}

watch(() => props.newModel, (newVal) => {
  localNewModel.value = { ...newVal }
}, { deep: true })

watch(localNewModel, (newVal) => {
  emit('update:newModel', newVal)
}, { deep: true })

function selectAvailableModel(model: any) {
  const newM = {
    name: model.name,
    filename: model.filename,
    port: props.state?.next_port || 8080,
    args: ''
  }
  emit('update:newModel', newM)
}

function submitModel() {
  emit('addModel')
}
</script>

<style scoped lang="postcss">
.model-manager-container {
  @apply grid grid-cols-1 lg:grid-cols-2 gap-6 mt-6;
}
.models-box {
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col h-[500px];
}
.models-title {
  @apply text-lg font-semibold text-white mb-4 flex justify-between items-center;
}
.models-count {
  @apply text-xs bg-gray-700 px-2 py-1 rounded text-gray-300;
}
.models-list-wrapper {
  @apply overflow-y-auto flex-1 pr-2;
}
.models-empty {
  @apply text-center text-gray-500 py-10;
}
.models-list {
  @apply space-y-3;
}
.model-item {
  @apply p-4 rounded-lg border transition-all;
}
.model-active {
  @apply bg-blue-900/20 border-blue-500/50;
}
.model-inactive {
  @apply bg-gray-900/50 border-gray-700 hover:border-gray-600;
}
.model-editing {
  @apply bg-gray-800 border-blue-500 ring-1 ring-blue-500;
}
.model-name {
  @apply font-medium text-white flex items-center gap-2;
}
.model-badge {
  @apply px-2 py-0.5 text-[10px] uppercase font-bold rounded-full bg-blue-500 text-white shrink-0;
}
.model-details {
  @apply text-xs text-gray-500 mt-1;
}
.model-actions {
  @apply flex gap-2 items-center;
}
.btn-start {
  @apply px-3 py-1.5 bg-gray-700 hover:bg-gray-600 text-white text-xs font-medium rounded transition-colors;
}
.btn-stop-local {
  @apply px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-xs font-medium rounded transition-colors;
}
.btn-edit {
  @apply text-xs px-2 py-1.5 text-blue-400 hover:text-blue-300 hover:bg-blue-400/10 rounded transition-colors focus:outline-none;
}
.btn-remove {
  @apply px-3 py-1.5 border border-red-500/30 text-red-400 hover:bg-red-500/10 text-xs font-medium rounded transition-colors;
}
.btn-remove-disabled {
  @apply opacity-50 cursor-not-allowed;
}
.add-title {
  @apply text-lg font-semibold text-white mb-4;
}
.add-form {
  @apply mb-6 space-y-3 p-4 bg-gray-900/50 rounded border border-gray-700;
}
.form-grid-2 {
  @apply grid grid-cols-2 gap-3;
}
.form-grid-3 {
  @apply grid grid-cols-3 gap-3;
}
.form-col-1 {
  @apply col-span-1;
}
.form-col-2 {
  @apply col-span-2;
}
.form-col-1-edit {
  @apply col-span-1;
}
.form-col-3-edit {
  @apply col-span-3;
}
.form-label {
  @apply block text-xs font-medium text-gray-400 mb-1;
}
.form-input {
  @apply w-full bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-sm text-white focus:border-blue-500 focus:outline-none;
}
.btn-add {
  @apply w-full bg-blue-600 hover:bg-blue-500 text-white py-2 rounded text-sm font-medium transition-colors mt-2;
}
.discovered-title {
  @apply text-sm font-semibold text-gray-300 mb-3;
}
.discovered-wrapper {
  @apply overflow-y-auto flex-1 pr-2;
}
.discovered-empty {
  @apply text-center text-gray-500 py-4 text-sm;
}
.discovered-list {
  @apply space-y-2;
}
.discovered-item {
  @apply p-3 bg-gray-700/30 rounded border border-gray-700 flex justify-between items-center hover:bg-gray-700/50 transition-colors;
}
.discovered-details {
  @apply truncate mr-4;
}
.discovered-name {
  @apply text-sm text-white font-medium truncate;
}
.discovered-file {
  @apply text-xs text-gray-500 truncate;
}
.btn-select {
  @apply px-2.5 py-1 bg-gray-600 hover:bg-gray-500 text-white text-xs rounded transition-colors opacity-0;
}
</style>
