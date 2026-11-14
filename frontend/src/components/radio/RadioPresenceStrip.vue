<script setup lang="ts">
import { computed } from "vue"
import { formatAbsoluteTime } from "../../utils/format/time"
import { RADIO_PRESENCE_CLOSER, RADIO_PRESENCE_DISTANCE, RADIO_PRESENCE_FARTHER, radioHourTitle, radioRangeLine } from "../../constants/radio"
import type { RadioPresenceDevice } from "../../types/radio"

// One device's last two days, an hour to a square: filled when the sensor heard it, outlined when it listened and did not, pale when it was not
// listening (so nothing is known). The squares are decoration; the summary line carries the meaning for a screen reader.
const props = defineProps<{ device: RadioPresenceDevice }>()
const heard = computed(() => props.device.hours.filter((h) => h.state === "here").length)
const summary = computed(() => `${props.device.label}: heard in ${heard.value} of the last ${props.device.hours.length} hours`)
// The distance strip: one bar an hour, from the nearest to the farthest the device could be (a log scale, closer at the top), for the hours it
// was heard with a measured signal. Hours with no reading leave a gap rather than a guess.
const W = 480
const H = 44
const MAX_M = 60
const slot = computed(() => W / Math.max(props.device.hours.length, 1))
const y = (m: number): number => 4 + ((H - 8) * Math.log(Math.min(MAX_M, Math.max(1, m)))) / Math.log(MAX_M)
const bars = computed(() => props.device.hours.flatMap((h, i) => (h.near_m && h.far_m ? [{ key: h.hour, x: i * slot.value, y1: y(h.near_m), y2: y(h.far_m), title: `${formatAbsoluteTime(h.hour)}: ${radioRangeLine(h.near_m, h.far_m)}` }] : [])))
const CELL: Record<string, string> = {
  here: "bg-state-success",
  away: "border border-muted bg-transparent",
  not_listening: "bg-hairline",
}
</script>

<template>
  <div class="flex flex-col gap-1" data-test="radio-presence">
    <div class="flex h-4 gap-px" role="img" :aria-label="summary">
      <span v-for="h in device.hours" :key="h.hour" class="h-full min-w-0 flex-1 rounded-[1px]" :class="CELL[h.state]" :title="radioHourTitle(formatAbsoluteTime(h.hour), h.state, h.signal)" data-test="radio-presence-hour" :data-state="h.state" />
    </div>
    <template v-if="bars.length">
      <svg :viewBox="`0 0 ${W} ${H}`" class="h-11 w-full text-hairline" preserveAspectRatio="none" role="img" :aria-label="RADIO_PRESENCE_DISTANCE" data-test="radio-presence-distance">
        <line x1="0" :x2="W" :y1="y(3)" :y2="y(3)" stroke="currentColor" stroke-dasharray="3 4" />
        <line x1="0" :x2="W" :y1="y(10)" :y2="y(10)" stroke="currentColor" stroke-dasharray="3 4" />
        <rect v-for="b in bars" :key="b.key" :x="b.x + 1" :y="b.y1" :width="Math.max(slot - 2, 1)" :height="Math.max(b.y2 - b.y1, 2)" rx="1" style="fill: rgb(var(--accent-brand))" opacity="0.7" data-test="radio-presence-bar"><title>{{ b.title }}</title></rect>
      </svg>
      <p class="m-0 flex justify-between text-[length:var(--text-micro)] text-muted"><span>{{ RADIO_PRESENCE_CLOSER }} ▲</span><span>{{ RADIO_PRESENCE_DISTANCE }}</span><span>▼ {{ RADIO_PRESENCE_FARTHER }}</span></p>
    </template>
  </div>
</template>
