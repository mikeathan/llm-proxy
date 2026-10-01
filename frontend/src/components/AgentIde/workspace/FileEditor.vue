<script setup lang="ts">
// The file editor surface: a plain monospace text area. The owning panel shows
// the file name, the unsaved state and Save; Ctrl/Cmd+S saves from here too.

const props = defineProps<{
  filename: string
  content: string
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'update:content', value: string): void
  (e: 'save'): void
}>()

const SAVE_KEY = 's'

function onKeydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === SAVE_KEY) {
    event.preventDefault()
    emit('save')
  }
}
</script>

<template>
  <div class="relative flex min-h-0 flex-1 flex-col">
    <div v-if="props.loading" role="status" class="absolute inset-0 z-[1] flex items-center justify-center bg-surface/80">
      <span class="font-mono text-[length:var(--text-small)] text-muted">loading…</span>
    </div>
    <textarea
      :value="props.content"
      :aria-label="`Contents of ${props.filename}`"
      spellcheck="false"
      class="min-h-[50vh] flex-1 resize-none bg-canvas p-4 font-mono text-[length:var(--text-small)] leading-[1.7] text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset"
      @input="emit('update:content', ($event.target as HTMLTextAreaElement).value)"
      @keydown="onKeydown"
    ></textarea>
  </div>
</template>
