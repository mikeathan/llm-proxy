<script setup lang="ts">
import { computed } from "vue"
import MicroLabel from "./MicroLabel.vue"

// Ruler meter: a continuous fill over a 1-bit dithered track with a tick every
// 10%. role="meter" with value attributes; the value is also written as text.
const props = withDefaults(
  defineProps<{ label: string; value: number; max?: number; state?: "info" | "running" | "success"; text?: string }>(),
  { max: 100, state: "info" },
)

const STATE_CLASS = { info: "text-accent-info", running: "text-state-running", success: "text-state-success" } as const
const PERCENT = 100

const percent = computed(() => Math.min(Math.max(props.value, 0), props.max) / props.max * PERCENT)
const valueText = computed(() => props.text ?? `${Math.round(percent.value)}%`)
</script>

<template>
  <div
    role="meter"
    :aria-label="label"
    :aria-valuenow="value"
    aria-valuemin="0"
    :aria-valuemax="max"
    :aria-valuetext="valueText"
    class="flex min-w-0 flex-col gap-1.5"
  >
    <div class="flex justify-between gap-2">
      <MicroLabel>{{ label }}</MicroLabel>
      <span class="font-mono text-[length:var(--text-small)] tabular-nums text-secondary">{{ valueText }}</span>
    </div>
    <div aria-hidden="true" :class="['meter-track h-2', STATE_CLASS[state]]">
      <span data-test="meter-fill" class="block h-full bg-current transition-[width] duration-slow ease-standard" :style="{ width: `${percent}%` }"></span>
    </div>
    <div aria-hidden="true" class="meter-scale h-1"></div>
  </div>
</template>

<style scoped>
.meter-track {
  background: repeating-conic-gradient(rgb(var(--bar-track)) 0 25%, transparent 0 50%) 0 0 / var(--dither-size) var(--dither-size);
}
.meter-scale {
  background: repeating-linear-gradient(to right, rgb(var(--text-decorative)) 0 1px, transparent 1px 10%);
}
</style>
