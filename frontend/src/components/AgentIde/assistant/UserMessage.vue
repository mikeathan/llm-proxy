<script setup lang="ts">
defineProps<{
  content: string
  timestamp: string
}>()

const emit = defineEmits<{
  retry: [text: string]
}>()
</script>

<template>
  <div class="user-message-wrapper">
    <div class="user-bubble">
      <div class="user-bubble-content">{{ content }}</div>
      <button
        class="user-retry-btn"
        :title="`Retry: ${content}`"
        @click="emit('retry', content)"
      >
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="1 4 1 10 7 10" />
          <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10" />
        </svg>
      </button>
    </div>
    <div class="user-timestamp">{{ timestamp }}</div>
  </div>
</template>

<style scoped>
.user-message-wrapper { @apply flex flex-col items-end gap-1; }
.user-bubble { @apply max-w-[85%] bg-blue-600 text-white rounded-2xl rounded-tr-sm px-4 py-2.5 shadow-sm relative; position: relative; }
.user-bubble-content { @apply text-sm leading-relaxed whitespace-pre-wrap break-words; }
.user-timestamp { @apply text-[10px] text-gray-500 pr-1; }
.user-retry-btn { @apply absolute -top-2 -right-2 w-6 h-6 rounded-full bg-gray-700 border border-gray-600 text-gray-300 flex items-center justify-center opacity-0 transition-opacity duration-150 hover:text-white hover:border-gray-500; }
.user-message-wrapper:hover .user-retry-btn { opacity: 1; }
</style>
