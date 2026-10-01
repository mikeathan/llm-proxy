<script setup lang="ts">
import { computed } from "vue"
import MicroLabel from "./MicroLabel.vue"

// Slot bar: one block per real slot, for discrete capacity (run lanes). Used
// slots fill with the state colour; free ones show the dithered track.
// role="meter" with the count also written as text, so colour never carries it alone.
const props = withDefaults(defineProps<{ label: string; used: number; limit: number; state?: "running" | "info" }>(), { state: "running" })

const STATE_CLASS = { running: "border-state-running bg-state-running", info: "border-accent-info bg-accent-info" } as const
const slots = computed(() => Array.from({ length: Math.max(props.limit, 0) }, (_, i) => i < props.used))
</script>

<template>
  <div
    role="meter"
    :aria-label="label"
    :aria-valuenow="used"
    aria-valuemin="0"
    :aria-valuemax="limit"
    :aria-valuetext="`${used} of ${limit} slots in use`"
    class="flex min-w-0 flex-col gap-1.5"
  >
    <div class="flex justify-between gap-2">
      <MicroLabel>{{ label }}</MicroLabel>
      <span class="font-mono text-[length:var(--text-small)] tabular-nums text-secondary">{{ used }}/{{ limit }}</span>
    </div>
    <div aria-hidden="true" class="flex h-3 gap-1">
      <span
        v-for="(filled, i) in slots"
        :key="i"
        data-test="slot"
        :data-used="filled"
        :class="['flex-1 border', filled ? STATE_CLASS[state] : 'slot-free border-strong']"
      ></span>
    </div>
  </div>
</template>

<style scoped>
.slot-free {
  background: repeating-conic-gradient(rgb(var(--bar-track)) 0 25%, transparent 0 50%) 0 0 / var(--dither-size) var(--dither-size);
}
</style>
