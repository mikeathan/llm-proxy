<script setup lang="ts">
import { computed } from "vue"
import { formatAbsoluteTime } from "../../utils/format/time"
import { RADIO_PRESENCE_NOW, radioHourTitle, radioPresenceAgo } from "../../constants/radio"
import { RADIO_PRESENCE_LEGEND_AWAY, RADIO_PRESENCE_LEGEND_HEARD, RADIO_PRESENCE_LEGEND_OFF } from "../../constants/radioIdentity"
import type { RadioPresenceDevice } from "../../types/radio"

// One device's last hours, an hour to a square, oldest on the left and now on the right (the axis says so): filled when the sensor heard it,
// outlined when it listened and did not, pale when it was not listening (so nothing is known). Filled is the brand ink, not the "healthy"
// green: a square says heard or not, never good or bad. The squares are decoration; the summary line carries the meaning for a screen reader.
// The hour-by-hour distance chart that was under it was dropped (2026-11-18): the owner could not read it, and the device page has the radar.
const props = defineProps<{ device: RadioPresenceDevice }>()
const heard = computed(() => props.device.hours.filter((h) => h.state === "here").length)
const summary = computed(() => `${props.device.label}: heard in ${heard.value} of the last ${props.device.hours.length} hours`)
const CELL: Record<string, string> = {
  here: "bg-accent-brand",
  away: "border border-muted bg-transparent",
  not_listening: "bg-hairline",
}
</script>

<template>
  <div class="flex flex-col gap-1" data-test="radio-presence">
    <div class="flex h-4 gap-px" role="img" :aria-label="summary">
      <span v-for="h in device.hours" :key="h.hour" class="h-full min-w-0 flex-1 rounded-[1px]" :class="CELL[h.state]" :title="radioHourTitle(formatAbsoluteTime(h.hour), h.state, h.signal)" data-test="radio-presence-hour" :data-state="h.state" />
    </div>
    <p class="m-0 flex justify-between text-[length:var(--text-micro)] text-muted" aria-hidden="true" data-test="radio-presence-axis"><span>{{ radioPresenceAgo(device.hours.length) }}</span><span>{{ RADIO_PRESENCE_NOW }}</span></p>
    <p class="m-0 flex flex-wrap gap-x-3 gap-y-0.5 text-[length:var(--text-micro)] text-muted" data-test="radio-presence-legend">
      <span class="inline-flex items-center gap-1"><span class="inline-block h-2.5 w-2.5 rounded-[1px] bg-accent-brand" aria-hidden="true" />{{ RADIO_PRESENCE_LEGEND_HEARD }}</span>
      <span class="inline-flex items-center gap-1"><span class="inline-block h-2.5 w-2.5 rounded-[1px] border border-muted" aria-hidden="true" />{{ RADIO_PRESENCE_LEGEND_AWAY }}</span>
      <span class="inline-flex items-center gap-1"><span class="inline-block h-2.5 w-2.5 rounded-[1px] bg-hairline" aria-hidden="true" />{{ RADIO_PRESENCE_LEGEND_OFF }}</span>
    </p>
  </div>
</template>
