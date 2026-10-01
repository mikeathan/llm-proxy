<script setup lang="ts">
import { computed } from "vue"

// Dot-matrix bar sparkline: each sample is a bar of stacked dots, the latest
// in full colour. An image with a text summary, so the trend is never
// conveyed visually alone.
const props = withDefaults(
  defineProps<{ values: number[]; label: string; unit: string; state?: "success" | "info" }>(),
  { state: "success" },
)

const PERCENT = 100
const STATE_CLASS = { success: "text-state-success", info: "text-accent-info" } as const

const peak = computed(() => Math.max(0, ...props.values))
const heights = computed(() => props.values.map((v) => (peak.value > 0 ? (v / peak.value) * PERCENT : 0)))
const summary = computed(() => {
  if (!props.values.length) return `${props.label}: no samples yet`
  const latest = props.values[props.values.length - 1]
  return `${props.label}: ${props.values.length} samples, latest ${latest} ${props.unit}, peak ${peak.value} ${props.unit}`
})
</script>

<template>
  <div role="img" :aria-label="summary" :class="['mt-2 flex h-7 items-end gap-0.5', STATE_CLASS[state]]">
    <span
      v-for="(height, i) in heights"
      :key="i"
      data-test="spark-bar"
      :data-latest="i === heights.length - 1 ? 'true' : undefined"
      :class="['spark-bar min-w-[3px] flex-1', i === heights.length - 1 ? 'opacity-100' : 'opacity-40']"
      :style="{ height: `${height}%` }"
    ></span>
  </div>
</template>

<style scoped>
.spark-bar {
  background: repeating-linear-gradient(to top, currentColor 0 3px, transparent 3px 5px);
}
</style>
