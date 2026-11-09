<script setup lang="ts">
import { computed } from "vue"
import { chartBars } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import { TRAFFIC_BUSIEST, TRAFFIC_CHART_HEIGHT, TRAFFIC_CHART_WIDTH } from "../../constants/netaudit"

// The bytes of each second of one look, as bars scaled so the busiest second fills the height. Drawn in SVG with the page's own colour, no
// chart library. It is an image to a screen reader with the busiest second as its name; the numbers are in the lists beside it.
const props = defineProps<{ perSecond: readonly number[] }>()
const SECONDS = 60
const MARKS = [0, 15, 30, 45, 60]
const chart = computed(() => chartBars(props.perSecond, TRAFFIC_CHART_WIDTH, TRAFFIC_CHART_HEIGHT))
const label = computed(() => {
  const b = chart.value.busiest
  const span = `Bytes each second over ${props.perSecond.length} seconds`
  return b ? `${span}; ${TRAFFIC_BUSIEST(b.second, formatBytes(b.bytes))}` : span
})
const mark = (s: number) => `${Math.floor(s / SECONDS)}:${String(s % SECONDS).padStart(2, "0")}`
const marks = computed(() => MARKS.filter((s) => s <= props.perSecond.length))
</script>

<template>
  <div v-if="chart.bars.length" class="flex flex-col gap-1">
    <svg :viewBox="`0 0 ${TRAFFIC_CHART_WIDTH} ${TRAFFIC_CHART_HEIGHT}`" width="100%" :height="TRAFFIC_CHART_HEIGHT" preserveAspectRatio="none" role="img" :aria-label="label" data-test="traffic-chart" class="text-accent-info-text">
      <rect v-for="b in chart.bars" :key="b.second" :x="b.x" :y="b.y" :width="b.width" :height="b.height" fill="currentColor" />
    </svg>
    <div class="flex justify-between text-[length:var(--text-micro)] text-muted" aria-hidden="true">
      <span v-for="s in marks" :key="s">{{ mark(s) }}</span>
    </div>
  </div>
</template>
