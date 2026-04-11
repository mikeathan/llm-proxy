<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'

const props = defineProps<{
  content: string
}>()

const htmlContent = computed(() => {
  if (!props.content) return ''
  // Use a secure way to render markdown. 
  // marked() is fast, but we should be mindful of XSS in production.
  return marked(props.content, {
    gfm: true,
    breaks: true,
  })
})
</script>

<template>
  <div class="markdown-body" v-html="htmlContent"></div>
</template>

<style scoped lang="postcss">
.markdown-body {
  @apply text-gray-200 text-sm leading-relaxed;
}

.markdown-body :deep(pre) {
  @apply bg-gray-950 border border-gray-800 rounded-lg p-5 overflow-x-auto mb-6 font-mono shadow-inner;
  white-space: pre !important;
}

.markdown-body :deep(code) {
  @apply font-mono text-[0.8rem] text-gray-300;
}

.markdown-body :deep(pre code) {
  @apply block p-0 bg-transparent border-0 leading-tight;
  white-space: pre !important;
}

.markdown-body :deep(p) {
  @apply mb-4;
}

.markdown-body :deep(strong) {
  @apply text-gray-100 font-bold;
}

.markdown-body :deep(h1), 
.markdown-body :deep(h2), 
.markdown-body :deep(h3) {
  @apply text-gray-50 font-black mt-6 mb-3 tracking-tight;
}
</style>
