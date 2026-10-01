<script setup lang="ts">
import type { DialogType } from "../../../types/ui"

// A standing notice inside a page (Phase 5 primitive): a tinted band with a
// state rule on its leading edge, a title and a body. Warning uses the
// attention hue (citrine, shared with "running"); text carries the meaning.
withDefaults(defineProps<{ tone?: DialogType; title: string }>(), { tone: "info" })
defineSlots<{ default?(): unknown; actions?(): unknown }>()

const TONE_CLASS: Record<DialogType, string> = {
  info: "border-accent-info bg-accent-info/[0.08]",
  warning: "border-state-running bg-state-running/[0.08]",
  error: "border-state-error bg-state-error/[0.08]",
}
const TITLE_CLASS: Record<DialogType, string> = {
  info: "text-accent-info-text",
  warning: "text-state-running",
  error: "text-state-error",
}
</script>

<template>
  <div :data-tone="tone" :class="['flex flex-col gap-1.5 border-l-2 px-3 py-2.5', TONE_CLASS[tone]]">
    <p :class="['m-0 text-[length:var(--text-small)] font-semibold', TITLE_CLASS[tone]]">{{ title }}</p>
    <div v-if="$slots.default" class="text-[length:var(--text-small)] text-secondary"><slot /></div>
    <div v-if="$slots.actions" class="mt-1 flex flex-wrap gap-2"><slot name="actions" /></div>
  </div>
</template>
