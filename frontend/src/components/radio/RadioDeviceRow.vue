<script setup lang="ts">
import { computed } from "vue"
import {
  RADIO_RADAR_MINE, RADIO_RADAR_NOT_HEARD, RADIO_RADAR_USUAL_ROW, RADIO_TREND_MARK, RADIO_TREND_WORDS, radioDbmLine, radioRangeLine,
} from "../../constants/radio"
import { deviceFacts, deviceMakerNote, deviceTags, deviceTitle, heardAgoWord, trailPoints } from "../../utils/radio/describe"
import type { RadioLiveDevice } from "../../types/radio"

// Radio · one row of the device list: what it is (the owner's name, the network it broadcasts, or its maker), the kind, Wi-Fi generation and channel,
// the distance range and signal, which way it is moving, how long ago it was heard and a small trail of where it was. The short id is the handle.
const props = defineProps<{ device: RadioLiveDevice; selected: boolean; now: number }>()
const emit = defineEmits<{ (e: "pick", id: string): void }>()
const TRAIL_W = 56
const TRAIL_H = 16
const facts = computed(() => deviceFacts(props.device))
const tags = computed(() => deviceTags(props.device))
const trail = computed(() => trailPoints(props.device.tail_m, TRAIL_W, TRAIL_H))
const heard = computed(() => (props.device.usual ? RADIO_RADAR_USUAL_ROW : props.device.heard ? heardAgoWord(props.device, props.now) : RADIO_RADAR_NOT_HEARD))
</script>

<template>
  <button type="button" class="flex min-h-11 w-full items-start justify-between gap-3 py-2 text-left hover:text-primary focus-visible:outline-none focus-visible:ring-2" :class="selected ? 'bg-surface-active' : ''" :aria-pressed="selected" data-test="radio-radar-row-button" @click="emit('pick', device.id)">
    <span class="flex min-w-0 flex-col gap-0.5">
      <span class="flex min-w-0 items-baseline gap-2">
        <span class="min-w-0 break-words" :class="device.mine ? 'text-primary' : 'text-secondary'" data-test="radio-row-title">{{ deviceTitle(device) }}</span>
        <span v-if="device.mine" class="text-[length:var(--text-small)] text-accent-brand">{{ RADIO_RADAR_MINE }}</span>
        <span v-if="!device.mine || device.label" class="flex-none font-mono text-[length:var(--text-small)] text-faint" data-test="radio-row-id">{{ device.id }}</span>
      </span>
      <span class="text-[length:var(--text-small)] text-muted" data-test="radio-row-facts">{{ facts.join(" · ") }}<template v-if="deviceMakerNote(device)"> · {{ deviceMakerNote(device) }}</template></span>
      <span v-if="tags.length" class="flex flex-wrap gap-1">
        <span v-for="t in tags" :key="t.text" class="rounded-full border px-2 text-[length:var(--text-small)]" :class="t.warn ? 'border-state-running text-state-running' : 'border-hairline text-muted'" data-test="radio-row-tag" :data-tag="t.test">{{ t.text }}</span>
      </span>
    </span>
    <span class="flex flex-none flex-col items-end gap-0.5 text-[length:var(--text-small)]">
      <span class="font-mono text-primary" data-test="radio-row-range">{{ radioRangeLine(device.near_m, device.far_m) }}</span>
      <span class="text-muted">{{ radioDbmLine(device.dbm) }}<template v-if="!device.usual"> · {{ RADIO_TREND_MARK[device.trend] }} {{ RADIO_TREND_WORDS[device.trend] }}</template></span>
      <svg v-if="trail" :viewBox="`0 0 ${TRAIL_W} ${TRAIL_H}`" class="h-4 w-14 text-accent-brand" role="img" aria-label="Where it was over the last readings" data-test="radio-row-trail"><polyline :points="trail" fill="none" stroke="currentColor" stroke-width="1.5" /></svg>
      <span class="text-faint" data-test="radio-row-heard">{{ heard }}</span>
    </span>
  </button>
</template>
