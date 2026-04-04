<template>
  <div class="model-manager-container">
    <!-- Configured Models List -->
    <div class="models-box">
      <h2 class="models-title">
        {{ filterProvider === 'local' ? 'Local Instances' : 'Cloud Models' }}
        <span class="models-count">{{ filteredModels.length }} Models</span>
      </h2>
      <div class="models-list-wrapper">
        <div v-if="filteredModels.length === 0" class="models-empty">
          No {{ filterProvider }} models configured.
        </div>
        <div class="models-list" v-else>
          <div v-for="model in filteredModels" :key="model.name"
               :class="['model-item', model.active ? 'model-active' : 'model-inactive', editingModel?.name === model.name ? 'model-editing' : '']">
            
            <!-- Normal View -->
            <div v-if="editingModel?.name !== model.name" class="w-full flex justify-between items-center gap-4">
              <div class="min-w-0 flex-1">
                <div class="model-name truncate" :title="model.name">
                  <span :class="['provider-badge', `badge-${model.provider}`]">{{ model.provider }}</span>
                  {{ model.name }}
                  <span v-if="model.active" class="model-badge-active">Online</span>
                </div>
                <div class="model-details truncate">
                  <template v-if="model.provider === 'local'">
                    Port: {{ model.port }} &bull; File: {{ model.filename }}
                  </template>
                  <template v-else>
                    Model ID: {{ model.model_id }}
                  </template>
                </div>
                <div class="model-details mt-1 truncate" v-if="model.args && model.args.length" :title="model.args.join(' ')">
                  Args: <span class="font-mono text-[10px] text-gray-400">{{ model.args.join(' ') }}</span>
                </div>
              </div>
              <div class="model-actions shrink-0">
                <!-- Start/Stop only for local -->
                <template v-if="model.provider === 'local'">
                  <button v-if="!model.active" @click="$emit('startModel', model.name)" class="btn-start">Start</button>
                  <button v-else @click="$emit('stopModel')" class="btn-stop-local">Stop</button>
                </template>
                
                <button @click="startEdit(model)" class="btn-edit">Edit</button>
                <button @click="$emit('removeModel', model.name)" :class="['btn-remove', model.active ? 'btn-remove-disabled' : '']" :disabled="model.active">Remove</button>
              </div>
            </div>

            <!-- Edit View -->
            <div v-else class="w-full flex flex-col gap-3">
              <div class="font-medium text-white mb-1">Edit {{ model.name }}</div>
              <div class="grid grid-cols-1 sm:grid-cols-4 gap-3">
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
                  <div class="sm:col-span-4">
                    <label class="form-label">Model ID</label>
                    <input v-model="editingModel.model_id" type="text" class="form-input">
                  </div>
                </template>
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

    <!-- Add Model Form -->
    <div class="models-box">
      <h2 class="add-title">Add New Model</h2>

      <form @submit.prevent="submitModel" class="add-form animate-in fade-in duration-500">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label class="form-label">Provider</label>
            <select v-model="localNewModel.provider" class="form-input">
              <template v-if="filterProvider === 'local'">
                <option value="local">Local Engine (Llama.cpp)</option>
              </template>
              <template v-else>
                <option value="gemini">Google Gemini</option>
                <option value="openai">OpenAI / Compatible</option>
                <option value="openrouter">OpenRouter</option>
                <option value="vertex">Google Vertex AI</option>
              </template>
            </select>
          </div>
          <div>
            <label class="form-label">Friendly Name</label>
            <input v-model="localNewModel.name" type="text" required placeholder="e.g. My-Model" class="form-input">
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3">
          <div v-if="localNewModel.provider === 'local'">
            <label class="form-label">Filename (.gguf)</label>
            <input v-model="localNewModel.filename" type="text" required placeholder="qwen2.5-3b.gguf" class="form-input">
          </div>
          <div v-else>
            <label class="form-label">Model ID</label>
            <input v-model="localNewModel.model_id" type="text" required placeholder="e.g. gpt-4o or gemini-1.5-pro" class="form-input">
          </div>
        </div>

        <div v-if="localNewModel.provider === 'local'" class="form-grid-3">
          <div class="form-col-1">
            <label class="form-label">Port</label>
            <input v-model.number="localNewModel.port" type="number" class="form-input">
          </div>
          <div class="form-col-2">
            <label class="form-label">Extra Args</label>
            <input v-model="localNewModel.args" type="text" placeholder="-c 4096" class="form-input font-mono">
          </div>
        </div>

        <button type="submit" class="btn-add">Add to Configuration</button>
      </form>

      <!-- Search Local Dir (Only for Local Tab) -->
      <template v-if="filterProvider === 'local'">
        <h3 class="discovered-title">Discovered in Directory</h3>
        <div class="discovered-wrapper">
          <div v-if="!availableModels || availableModels.length === 0" class="discovered-empty">
            No new .gguf files found.
          </div>
          <div class="discovered-list" v-else>
            <div v-for="model in availableModels" :key="model.filename" class="discovered-item group">
              <div class="discovered-details">
                <div class="discovered-name">{{ model.name }}</div>
                <div class="discovered-file">{{ model.filename }}</div>
              </div>
              <button @click="selectAvailableModel(model)" class="btn-select group-hover:opacity-100">Select</button>
            </div>
          </div>
        </div>
      </template>
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
  filterProvider: 'local' | 'cloud'
}>()

const emit = defineEmits<{
  (e: 'update:newModel', model: any): void
  (e: 'startModel', name: string): void
  (e: 'stopModel'): void
  (e: 'removeModel', name: string): void
  (e: 'updateModel', model: any): void
  (e: 'addModel', model: any): void
}>()

const localNewModel = ref({ ...props.newModel })
const editingModel = ref<any>(null)

const filteredModels = computed(() => {
  if (!props.state?.models) return []
  if (props.filterProvider === 'local') {
    return props.state.models.filter(m => m.provider === 'local')
  }
  return props.state.models.filter(m => m.provider !== 'local')
})

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
  if (props.filterProvider !== 'local') return
  localNewModel.value = {
    ...localNewModel.value,
    provider: 'local',
    name: model.name,
    filename: model.filename,
    port: props.state?.next_port || 8000,
    args: ''
  }
}

watch(() => props.filterProvider, (tab) => {
  if (tab === 'local') {
    localNewModel.value.provider = 'local'
  } else if (localNewModel.value.provider === 'local') {
    localNewModel.value.provider = 'gemini'
  }
}, { immediate: true })

function submitModel() {
  emit('addModel', localNewModel.value)
}
</script>

<style scoped lang="postcss">
.model-manager-container {
  @apply grid grid-cols-1 lg:grid-cols-2 gap-6;
}
.models-box {
  @apply bg-gray-800 rounded-lg shadow-xl border border-gray-700 p-5 flex flex-col min-h-[300px] h-[500px];
}
.models-title {
  @apply text-lg font-semibold text-white mb-4 flex justify-between items-center;
}
.models-count {
  @apply text-[10px] bg-gray-700 px-2 py-0.5 rounded text-gray-400;
}
.models-list-wrapper {
  @apply overflow-y-auto flex-1 pr-2;
}
.models-empty {
  @apply text-center text-gray-500 py-10 italic text-sm;
}
.models-list {
  @apply space-y-3;
}
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
.model-name {
  @apply font-semibold text-white flex items-center gap-2;
}
.provider-badge {
  @apply px-1.5 py-0.5 rounded text-[9px] uppercase font-bold tracking-tight border;
}
.badge-local { @apply bg-blue-900/30 text-blue-400 border-blue-500/30; }
.badge-gemini { @apply bg-purple-900/30 text-purple-400 border-purple-500/30; }
.badge-openai { @apply bg-green-900/30 text-green-400 border-green-500/30; }

.model-badge-active {
  @apply px-1.5 py-0.5 text-[9px] uppercase font-black rounded-full bg-green-500 text-gray-950 shadow-[0_0_8px_rgba(34,197,94,0.4)];
}
.model-details {
  @apply text-[11px] text-gray-500 mt-1;
}
.model-actions {
  @apply flex gap-2 items-center flex-wrap justify-end shrink-0;
}
.btn-start {
  @apply px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-[11px] font-bold rounded shadow-lg transition-all active:scale-95;
}
.btn-stop-local {
  @apply px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-[11px] font-bold rounded shadow-lg transition-all active:scale-95;
}
.btn-edit {
  @apply text-[11px] px-2 py-1.5 text-gray-400 hover:text-white transition-colors;
}
.btn-remove {
  @apply px-2 py-1.5 text-[11px] text-gray-500 hover:text-red-400 transition-colors;
}
.btn-remove-disabled {
  @apply opacity-20 cursor-not-allowed grayscale;
}
.add-title {
  @apply text-lg font-semibold text-white mb-4;
}
.add-form {
  @apply mb-6 space-y-4 p-5 bg-gray-900 border border-gray-700 rounded-lg shadow-inner;
}
.form-grid-3 {
  @apply grid grid-cols-1 sm:grid-cols-3 gap-3;
}
.form-col-1 {
  @apply sm:col-span-1;
}
.form-col-2 {
  @apply sm:col-span-2;
}
.form-label {
  @apply block text-[10px] uppercase font-bold text-gray-500 mb-1.5 tracking-wider;
}
.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}
select.form-input {
  @apply appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236b7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22m6%208%204%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[position:right_0.5rem_center] bg-[length:1.25rem_1.25rem] bg-no-repeat pr-10;
}

.btn-add {
  @apply w-full bg-blue-700 hover:bg-blue-600 text-white py-2.5 rounded-md text-xs font-black uppercase tracking-widest transition-all mt-2 shadow-lg hover:shadow-blue-600/20 active:scale-[0.98];
}
.discovered-title {
  @apply text-xs font-bold text-gray-500 uppercase tracking-widest mb-3 px-1;
}
.discovered-wrapper {
  @apply overflow-y-auto flex-1 pr-1;
}
.discovered-empty {
  @apply text-center text-gray-600 py-4 text-xs italic;
}
.discovered-list {
  @apply space-y-1.5;
}
.discovered-item {
  @apply p-2.5 bg-gray-900/50 rounded-md border border-gray-700/50 flex justify-between items-center transition-all hover:border-gray-600;
}
.discovered-details {
  @apply truncate mr-4;
}
.discovered-name {
  @apply text-xs text-gray-200 font-bold truncate;
}
.discovered-file {
  @apply text-[10px] text-gray-600 truncate font-mono;
}
.btn-select {
  @apply px-3 py-1 bg-gray-700 hover:bg-blue-600 text-white text-[10px] font-bold rounded transition-all opacity-0 scale-95;
}
</style>

