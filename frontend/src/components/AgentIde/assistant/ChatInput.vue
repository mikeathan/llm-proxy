<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import Icon from '../../icons/Icon.vue'
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
      <ArcOrbitLoader :active="loading && paused" :thickness="1" radius="0.75rem" />
      <textarea
        ref="inputRef"
        :value="inputMessage"
        @input="emit('update:inputMessage', ($event.target as HTMLTextAreaElement).value)"
        @keydown="onKeydown"
        placeholder="Ask the workspace agent..."
        class="chat-input"
        :class="{ 'is-loading': loading }"
        rows="1"
        :disabled="loading"
      ></textarea>
    </div>
    <button
      v-if="loading"
      @click="emit('cancel')"
      class="btn-stop"
      title="Stop"
    >
      <Icon name="close" size="md" />
    </button>
    <button
      v-else
      @click="emit('send')"
      :disabled="!inputMessage.trim()"
      class="btn-send"
    >
      <Icon name="send" size="md" />
    </button>
  </div>
</template>

<style scoped>
.input-area { @apply p-3 sm:p-4 border-t border-gray-700 bg-gray-800 flex gap-2 shrink-0; }
.input-wrap { @apply flex-1 relative; isolation: isolate; }
.chat-input { @apply w-full bg-gray-900 border border-gray-700 rounded-xl px-4 py-3 text-sm text-gray-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 resize-none placeholder-gray-600 transition-colors; position: relative; z-index: 0; }
.chat-input:disabled { @apply opacity-50 cursor-not-allowed; }
.chat-input.is-loading { @apply border-transparent; }
.btn-send { @apply bg-blue-600 hover:bg-blue-500 text-white rounded-xl px-4 flex items-center justify-center transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-md w-14 shrink-0; }
.btn-stop { @apply bg-red-700 hover:bg-red-600 text-white rounded-xl px-4 flex items-center justify-center transition-colors shadow-md w-14 shrink-0; }
</style>
