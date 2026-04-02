<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  file: { workspace: string, filename: string }
  content: string
  loading: boolean
  saving: boolean
}>()

const emit = defineEmits<{
  (e: 'update:content', value: string): void
  (e: 'save'): void
}>()

const localContent = computed({
  get: () => props.content,
  set: (val) => emit('update:content', val)
})
</script>

<template>
  <div class="h-full flex flex-col">
    <div class="p-3 border-b border-gray-700 flex justify-between items-center bg-gray-900">
      <div class="flex items-center gap-2">
        <span class="text-gray-400 text-xs">📁 {{ file.workspace }}</span>
        <span class="text-gray-500">/</span>
        <span class="font-medium text-sm text-gray-200">📄 {{ file.filename }}</span>
      </div>
      <button 
        @click="emit('save')" 
        :disabled="saving || loading"
        class="bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white px-3 py-1.5 rounded text-xs font-medium"
      >
        {{ saving ? 'Saving...' : 'Save File' }}
      </button>
    </div>
    <div class="flex-1 relative">
      <div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-gray-800/80 z-10">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>
      <textarea 
        v-model="localContent" 
        class="w-full h-full bg-gray-900 text-gray-300 font-mono text-sm p-4 focus:outline-none resize-none leading-relaxed"
        spellcheck="false"
      ></textarea>
    </div>
  </div>
</template>
