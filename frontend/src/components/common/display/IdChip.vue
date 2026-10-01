<script setup lang="ts">
import { computed, onUnmounted, ref } from "vue"

// A truncated mono ID. The full value is in the tooltip and accessible name;
// clicking copies it, with the result announced politely.
const props = withDefaults(defineProps<{ id: string; visible?: number }>(), { visible: 8 })

const FEEDBACK_MS = 2000
const COPIED = "Copied"
const COPY_FAILED = "Copy failed"

const short = computed(() => (props.id.length > props.visible ? `${props.id.slice(0, props.visible)}…` : props.id))
const feedback = ref("")
let timer: ReturnType<typeof setTimeout> | null = null

async function copy() {
  try {
    await navigator.clipboard.writeText(props.id)
    feedback.value = COPIED
  } catch {
    feedback.value = COPY_FAILED
  }
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => (feedback.value = ""), FEEDBACK_MS)
}
onUnmounted(() => {
  if (timer) clearTimeout(timer)
})
</script>

<template>
  <span class="inline-flex items-center gap-2">
    <button
      type="button"
      :title="id"
      :aria-label="`Copy ID ${id}`"
      class="inline-flex h-[22px] cursor-copy items-center rounded-[2px] border border-hairline bg-surface-raised px-[7px] font-mono text-[length:var(--text-small)] text-secondary hover:border-control focus-visible:outline-none focus-visible:ring-2"
      @click="copy"
    >{{ short }}</button>
    <span role="status" class="font-mono text-[length:var(--text-micro)] text-muted">{{ feedback }}</span>
  </span>
</template>
