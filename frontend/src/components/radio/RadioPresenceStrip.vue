<script setup lang="ts">
import { computed } from "vue"
import { formatAbsoluteTime } from "../../utils/format/time"
import { radioHourTitle } from "../../constants/radio"
import type { RadioPresenceDevice } from "../../types/radio"

// One device's last two days, an hour to a square: filled when the sensor heard it, outlined when it listened and did not, pale when it was not
// listening (so nothing is known). The squares are decoration; the summary line carries the meaning for a screen reader.
const props = defineProps<{ device: RadioPresenceDevice }>()
const heard = computed(() => props.device.hours.filter((h) => h.state === "here").length)
const summary = computed(() => `${props.device.label}: heard in ${heard.value} of the last ${props.device.hours.length} hours`)
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
  </div>
</template>
