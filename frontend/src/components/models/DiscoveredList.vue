<script setup lang="ts">
import type { AvailableModel } from '../../types'

defineProps<{
  availableModels: AvailableModel[]
}>()

const emit = defineEmits<{
  (e: 'select', model: AvailableModel): void
}>()
</script>

<template>
  <div class="discovered-section">
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
          <button @click="emit('select', model)" class="btn-select group-hover:opacity-100">Select</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
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
