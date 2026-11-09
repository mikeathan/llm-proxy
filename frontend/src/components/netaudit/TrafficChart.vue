<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { chartBars, secondAt, secondClock } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import { TRAFFIC_BUSIEST, TRAFFIC_CHART_HEIGHT, TRAFFIC_CHART_WIDTH, TRAFFIC_SECOND_PICK, TRAFFIC_SECOND_READOUT } from "../../constants/netaudit"

// The bytes of each second of one look, as bars scaled so the busiest second fills the height. Drawn in SVG with the page's own colour, no
// chart library. It is an image to a screen reader with the busiest second as its name; the numbers are in the lists beside it. A click on a
// bar, or the slider under the chart (which a keyboard and a thumb can use), reads out that one second's bytes. Only per-second totals are
// kept, so there is nothing deeper to open.
const props = defineProps<{ perSecond: readonly number[] }>()
const MARKS = [0, 15, 30, 45, 60]
const NONE = -1
const DIMMED = 0.35
const chart = computed(() => chartBars(props.perSecond, TRAFFIC_CHART_WIDTH, TRAFFIC_CHART_HEIGHT))
const label = computed(() => {
  const b = chart.value.busiest
  const span = `Bytes each second over ${props.perSecond.length} seconds`
  return b ? `${span}; ${TRAFFIC_BUSIEST(b.second, formatBytes(b.bytes))}` : span
})
const marks = computed(() => MARKS.filter((s) => s <= props.perSecond.length))
const picked = ref(NONE)
// A new look has new seconds, so the old pick means nothing.
watch(() => props.perSecond, () => { picked.value = NONE })
const readout = computed(() => (picked.value === NONE ? "" : TRAFFIC_SECOND_READOUT(secondClock(picked.value), formatBytes(props.perSecond[picked.value] ?? 0))))
const opacityOf = (second: number) => (picked.value === NONE || picked.value === second ? 1 : DIMMED)

function pick(event: MouseEvent) {
  const box = (event.currentTarget as SVGElement).getBoundingClientRect()
  picked.value = secondAt(event.clientX - box.left, box.width, props.perSecond.length)
}
</script>

<template>
  <div v-if="chart.bars.length" class="flex flex-col gap-1">
    <svg :viewBox="`0 0 ${TRAFFIC_CHART_WIDTH} ${TRAFFIC_CHART_HEIGHT}`" width="100%" :height="TRAFFIC_CHART_HEIGHT" preserveAspectRatio="none" role="img" :aria-label="label" data-test="traffic-chart" class="cursor-pointer text-accent-info-text" @click="pick">
      <rect v-for="b in chart.bars" :key="b.second" :x="b.x" :y="b.y" :width="b.width" :height="b.height" fill="currentColor" :opacity="opacityOf(b.second)" />
    </svg>
    <div class="flex justify-between text-[length:var(--text-micro)] text-muted" aria-hidden="true">
      <span v-for="s in marks" :key="s">{{ secondClock(s) }}</span>
    </div>
    <input
      type="range"
      min="0"
      :max="perSecond.length - 1"
      :value="picked === NONE ? 0 : picked"
      :aria-label="TRAFFIC_SECOND_PICK"
      class="w-full max-sm:h-11"
      data-test="traffic-second-pick"
      @input="picked = Number(($event.target as HTMLInputElement).value)"
    />
    <p v-if="readout" class="m-0 text-[length:var(--text-small)] text-secondary" role="status" data-test="traffic-second">{{ readout }}</p>
  </div>
</template>
