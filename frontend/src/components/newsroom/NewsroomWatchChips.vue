<script setup lang="ts">
import { FILTER_ALL_WATCHES, FILTER_WATCH_LABEL } from "../../constants/newsroom"
import type { NewsroomWatch } from "../../types/newsroom"

// Which watch the feed or the dashboard card shows: one chip per watch after "All watches", the chosen one outlined. It is the same look as the Radio
// filter chips (aria-pressed buttons in a group). There is no count on a chip: the feed is fetched already narrowed to the chosen watch, so the others
// would all read 0. A long watch name is cut short on the chip and whole in its tooltip.
defineProps<{ watches: readonly NewsroomWatch[]; modelValue: number | null }>()
const emit = defineEmits<{ (e: "update:modelValue", value: number | null): void }>()
const CHIP = "inline-flex min-h-11 max-w-full items-center rounded-[var(--radius-md)] border px-3 text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8"
</script>

<template>
  <div role="group" :aria-label="FILTER_WATCH_LABEL" class="flex flex-wrap gap-1.5" data-test="watch-chips">
    <button type="button" :aria-pressed="modelValue === null" :class="[CHIP, modelValue === null ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover']" @click="emit('update:modelValue', null)">{{ FILTER_ALL_WATCHES }}</button>
    <button
      v-for="w in watches"
      :key="w.id"
      type="button"
      :aria-pressed="modelValue === w.id"
      :title="w.name"
      :class="[CHIP, modelValue === w.id ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover']"
      @click="emit('update:modelValue', w.id)"
    ><span class="truncate">{{ w.name }}</span></button>
  </div>
</template>
