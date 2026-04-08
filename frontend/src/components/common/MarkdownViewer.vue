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

<style scoped>
.markdown-body {
  color: #e5e7eb;
  font-size: 0.875rem;
  line-height: 1.6;
}

.markdown-body :deep(pre) {
  background-color: #030712;
  border: 1px solid #1f2937;
  border-radius: 0.5rem;
  padding: 1.25rem;
  overflow-x: auto;
  margin-bottom: 1.5rem;
  white-space: pre !important;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  box-shadow: inset 0 2px 4px 0 rgba(0, 0, 0, 0.05);
}

.markdown-body :deep(code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 0.8rem;
  color: #d1d5db;
}

.markdown-body :deep(pre code) {
  display: block;
  padding: 0;
  background-color: transparent;
  border: 0;
  white-space: pre !important;
  line-height: 1.25;
}

.markdown-body :deep(p) {
  margin-bottom: 1rem;
}

.markdown-body :deep(strong) {
  color: #f3f4f6;
  font-weight: 700;
}

.markdown-body :deep(h1), 
.markdown-body :deep(h2), 
.markdown-body :deep(h3) {
  color: #f9fafb;
  font-weight: 800;
  margin-top: 1.5rem;
  margin-bottom: 0.75rem;
  letter-spacing: -0.025em;
}
</style>
