<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useRadioAir } from "../../composables/radio/useRadioAir"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  RADIO_CHANGES_TITLE, RADIO_GUIDE_DIRECTION, RADIO_GUIDE_MOTION, RADIO_GUIDE_TITLE, RADIO_RADAR_CALIBRATED, RADIO_RADAR_PROVISIONAL, RADIO_RADAR_USUAL_LEGEND,
  radioChangeLine, radioListenedLine, radioProvisionalLine,
} from "../../constants/radio"

// Radio · How to read this: the notes that used to sit as four paragraphs under the radar, folded into one disclosure. Advanced adds how the sorting
// rules stand and what changed in how devices were sorted.
defineProps<{ calibrated: boolean; hasUsual: boolean }>()
const a = useRadioAir()
const { advanced } = useNetauditViewMode()
const air = computed(() => a.air.value)
onMounted(() => { void a.load() })
</script>

<template>
  <details class="text-[length:var(--text-small)] text-muted" data-test="radio-guide">
    <summary class="inline-flex min-h-11 cursor-pointer items-center text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2">{{ RADIO_GUIDE_TITLE }}</summary>
    <div class="flex flex-col gap-2 pb-2">
      <p class="m-0">{{ RADIO_GUIDE_DIRECTION }}</p>
      <p class="m-0" data-test="radio-radar-model">{{ calibrated ? RADIO_RADAR_CALIBRATED : RADIO_RADAR_PROVISIONAL }}</p>
      <p v-if="hasUsual" class="m-0" data-test="radio-radar-legend">{{ RADIO_RADAR_USUAL_LEGEND }}</p>
      <p class="m-0">{{ RADIO_GUIDE_MOTION }}</p>
      <template v-if="advanced && air">
        <p class="m-0" data-test="radio-around-listened">{{ radioListenedLine(air.listened_cycles, air.rules.min_cycles_listened, air.rules.window_days) }}</p>
        <p v-if="air.rules.provisional" class="m-0" data-test="radio-around-provisional">{{ radioProvisionalLine(advanced, air.rules.version) }}</p>
        <div v-if="air.changes.length" class="flex flex-col gap-1">
          <h3 class="m-0 font-medium text-secondary">{{ RADIO_CHANGES_TITLE }}</h3>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="c in air.changes.slice(0, 6)" :key="c.day + c.from + c.to + c.reason" class="py-1.5 text-secondary" data-test="radio-change"><span class="text-muted">{{ c.day }}</span> · {{ radioChangeLine(c.n, c.from, c.to, c.reason) }}</li>
          </ul>
        </div>
      </template>
    </div>
  </details>
</template>
