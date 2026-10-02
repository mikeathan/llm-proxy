<script setup lang="ts">
import { onUnmounted, ref } from 'vue'
import BaseButton from '../buttons/BaseButton.vue'

// Copies a value (text as-is, anything else as pretty JSON). Icon-only, named
// by `title`: a copy icon that turns into a green check for a moment once
// copied; the result is announced politely, like IdChip.
const props = defineProps<{
  text: unknown
  /** Accessible name and tooltip. */
  title?: string
}>()

const FEEDBACK_MS = 2000
const COPIED = 'Copied'
const COPY_FAILED = 'Copy failed'
const DEFAULT_LABEL = 'Copy to clipboard'

const feedback = ref('')
let timer: ReturnType<typeof setTimeout> | null = null

async function copy() {
  try {
    await navigator.clipboard.writeText(typeof props.text === 'string' ? props.text : JSON.stringify(props.text, null, 2))
    feedback.value = COPIED
  } catch {
    feedback.value = COPY_FAILED
  }
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => (feedback.value = ''), FEEDBACK_MS)
}
onUnmounted(() => {
  if (timer) clearTimeout(timer)
})
</script>

<template>
  <span class="inline-flex flex-none items-center gap-1">
    <BaseButton
      variant="ghost"
      size="sm"
      :icon="feedback === COPIED ? 'check' : 'copy'"
      icon-only
      :label="title || DEFAULT_LABEL"
      :class-name="feedback === COPIED ? '!text-state-success' : ''"
      @click.stop.prevent="copy"
    />
    <span role="status" class="font-mono text-[length:var(--text-micro)] text-muted">{{ feedback }}</span>
  </span>
</template>
