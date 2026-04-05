<script setup lang="ts">
import { ref, computed } from 'vue'
import { argsToString, stringToArgs } from '../../utils/config'
import type { Model } from '../../types'

const props = defineProps<{
  model: Model
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
}

const editingArgsStr = computed({
  get: () => argsToString(editingModel.value?.args),
  set: (val: string) => {
    if (editingModel.value) editingModel.value.args = stringToArgs(val)
  }
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
    <div v-if="!isEditing" class="w-full flex justify-between items-center gap-4">
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
          <button v-if="!model.active" @click="$emit('start-model', model.name)" class="btn-start">Start</button>
          <button v-else @click="$emit('stop-model')" class="btn-stop-local">Stop</button>
        </template>
        
        <button @click="initializeEdit(); $emit('start-edit', model)" class="btn-edit">Edit</button>
        <button @click="$emit('remove-model', model.name)" :class="['btn-remove', model.active ? 'btn-remove-disabled' : '']" :disabled="model.active">Remove</button>
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
        <button @click="$emit('cancel-edit')" class="btn-remove">Cancel</button>
        <button @click="saveEdit" class="btn-start !bg-blue-600 hover:!bg-blue-500">Save Changes</button>
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
.model-name {
  @apply font-semibold text-white flex items-center gap-2;
}
.provider-badge {
  @apply px-1.5 py-0.5 rounded text-[9px] uppercase font-bold tracking-tight border;
}
.badge-local { @apply bg-blue-900/30 text-blue-400 border-blue-500/30; }
.badge-gemini { @apply bg-purple-900/30 text-purple-400 border-purple-500/30; }
.badge-openai { @apply bg-green-900/30 text-green-400 border-green-500/30; }
.badge-openrouter { @apply bg-orange-900/30 text-orange-400 border-orange-500/30; }
.badge-vertex { @apply bg-red-900/30 text-red-400 border-red-500/30; }

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

.form-label {
  @apply block text-[10px] uppercase font-bold text-gray-500 mb-1.5 tracking-wider;
}
.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}
.form-col-1-edit {
  @apply sm:col-span-1;
}
.form-col-3-edit {
  @apply sm:col-span-3;
}
</style>
