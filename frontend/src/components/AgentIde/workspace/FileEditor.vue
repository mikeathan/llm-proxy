<script setup lang="ts">
import { ref, watch } from 'vue'

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

const textareaRef = ref<HTMLTextAreaElement | null>(null)

watch(() => props.content, (val) => {
  if (textareaRef.value && textareaRef.value.value !== val) {
    textareaRef.value.value = val
  }
})
</script>

<template>
  <div class="editor-shell">
    <div class="editor-toolbar">
      <div class="toolbar-path">
        <span class="path-workspace">📁 {{ file.workspace }}</span>
        <span class="path-divider">/</span>
        <span class="path-filename">📄 {{ file.filename }}</span>
      </div>
      <button 
        @click="emit('save')" 
        :disabled="saving || loading"
        class="btn-save"
      >
        {{ saving ? 'Saving...' : 'Save File' }}
      </button>
    </div>
    <div class="editor-content-area">
      <div v-if="loading" class="loader-overlay">
        <div class="spinner"></div>
      </div>
      <textarea 
        ref="textareaRef"
        :value="props.content"
        @input="emit('update:content', ($event.target as HTMLTextAreaElement).value)"
        class="editor-textarea"
        spellcheck="false"
      ></textarea>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.editor-shell {
  @apply flex-1 flex flex-col;
}

.editor-toolbar {
  @apply p-3 border-b border-gray-700 flex justify-between items-center bg-gray-900;
}

.toolbar-path {
  @apply flex items-center gap-2;
}

.path-workspace {
  @apply text-gray-400 text-xs;
}

.path-divider {
  @apply text-gray-500;
}

.path-filename {
  @apply font-medium text-sm text-gray-200;
}

.btn-save {
  @apply bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white px-3 py-1.5 rounded text-xs font-medium transition-colors;
}

.editor-content-area {
  @apply flex-1 relative;
}

.loader-overlay {
  @apply absolute inset-0 flex items-center justify-center bg-gray-800/80 z-10;
}

.spinner {
  @apply animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500;
}

.editor-textarea {
  @apply w-full h-full bg-gray-900 text-gray-300 font-mono text-sm p-4 focus:outline-none resize-none leading-relaxed;
}
</style>
