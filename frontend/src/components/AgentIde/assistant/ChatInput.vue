<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import ArcOrbitLoader from '../../common/layout/ArcOrbitLoader.vue'

defineProps<{
  loading: boolean
  paused: boolean
  inputMessage: string
}>()

const emit = defineEmits<{
  send: []
  cancel: []
  'update:inputMessage': [value: string]
}>()

const inputRef = ref<HTMLTextAreaElement | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    emit('send')
  }
}

function onGlobalKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault()
    inputRef.value?.focus()
  }
}

onMounted(() => document.addEventListener('keydown', onGlobalKeydown))
onUnmounted(() => document.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <div class="input-area">
    <div class="input-wrap">
      <ArcOrbitLoader :active="loading && paused" :thickness="1" />
      <textarea
        ref="inputRef"
        aria-label="Message the assistant"
        :value="inputMessage"
        @input="emit('update:inputMessage', ($event.target as HTMLTextAreaElement).value)"
        @keydown="onKeydown"
        placeholder="Ask the workspace agent…"
        class="chat-input"
        :class="{ 'is-loading': loading }"
        rows="1"
        :disabled="loading"
      ></textarea>
    </div>
    <BaseButton v-if="loading" variant="danger" size="lg" icon="stop" icon-only label="Stop the run" @click="emit('cancel')" />
    <BaseButton v-else variant="primary" size="lg" icon="send" icon-only label="Send" :disabled="!inputMessage.trim()" @click="emit('send')" />
  </div>
</template>

<style scoped>
.input-area { @apply p-3 sm:p-4 border-t border-hairline bg-surface flex items-end gap-2 shrink-0; }
.input-wrap { @apply flex-1 relative; isolation: isolate; }
.chat-input { @apply block w-full rounded-[var(--radius-md)] border border-control bg-canvas px-3 py-2.5 text-[length:var(--text-body)] text-primary placeholder:text-faint resize-none transition-colors focus-visible:outline-none focus-visible:ring-2; position: relative; z-index: 0; }
.chat-input:disabled { @apply opacity-50 cursor-not-allowed; }
.chat-input.is-loading { @apply border-transparent; }

</style>
