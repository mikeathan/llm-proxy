<script setup lang="ts">
import { computed } from "vue"

// Shape-matched dithered placeholders and a block-caret loader; the label is
// announced to assistive tech. Motion follows the tokens (zero when reduced).
const props = withDefaults(defineProps<{ label: string; rows?: number }>(), { rows: 4 })
const ROW_STEP = 12
const widths = computed(() => Array.from({ length: props.rows }, (_, i) => `${90 - i * ROW_STEP}%`))
</script>

<template>
  <div role="status" aria-live="polite" class="flex w-full flex-col items-start gap-2 px-2 py-8">
    <span aria-hidden="true" class="font-mono text-[length:var(--text-small)] text-muted">loading<span class="loading-caret ml-0.5 text-accent-brand">▌</span></span>
    <span class="sr-only">{{ label }}</span>
    <div class="flex w-full flex-col gap-2.5" aria-hidden="true">
      <span v-for="(width, i) in widths" :key="i" data-test="skeleton" class="dither block h-2.5" :style="{ width }"></span>
    </div>
  </div>
</template>

<style scoped>
.dither {
  background: repeating-conic-gradient(rgb(var(--bar-track)) 0 25%, transparent 0 50%) 0 0 / var(--dither-size) var(--dither-size);
}
.loading-caret {
  animation: blink calc(var(--motion-slow) * 3) steps(1) infinite;
}
@keyframes blink {
  50% {
    opacity: 0;
  }
}
</style>
