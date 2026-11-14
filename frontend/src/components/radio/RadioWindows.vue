<script setup lang="ts">
import { computed } from "vue"
import { formatTime } from "../../utils/format/time"
import {
  RADIO_WINDOWS_EMPTY, RADIO_WINDOWS_TITLE, RADIO_WINDOW_CUT, RADIO_WINDOW_CUT_BADGE, RADIO_WINDOW_MISSED, RADIO_WINDOW_OFF_CHANNEL, RADIO_WINDOW_ON_CHANNEL,
  radioBand, radioChannelWord, radioWindowFrames, radioWindowOwn,
} from "../../constants/radio"
import type { RadioWindow } from "../../types/radio"

// Radio · Last windows: one row per finished window, newest first. A window is a short listen on one channel; the row says when, where (channel and
// band), how much was heard, and, as a plain badge, that a busy channel was cut at the frame limit so its count is a floor.
const props = defineProps<{ windows: RadioWindow[] }>()
const SHOWN = 12
const rows = computed(() => [...props.windows].reverse().slice(0, SHOWN))

function note(w: RadioWindow): string {
  if (w.frames === 0) return w.stop.replace(/_/g, " ")
  return w.channel_matched ? RADIO_WINDOW_ON_CHANNEL : RADIO_WINDOW_OFF_CHANNEL
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_WINDOWS_TITLE }}</h3>
    <p v-if="!rows.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-windows-empty">{{ RADIO_WINDOWS_EMPTY }}</p>
    <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
      <li v-for="w in rows" :key="w.at + w.channel" class="grid grid-cols-[5.25rem_minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-0.5 py-1.5 text-[length:var(--text-small)]" data-test="radio-window" :data-channel="w.channel">
        <span class="whitespace-nowrap font-mono text-muted">{{ formatTime(w.at) }}</span>
        <span class="min-w-0"><span class="text-primary" data-test="radio-window-channel">{{ radioChannelWord(w.channel) }}</span><span class="ml-2 text-muted" data-test="radio-window-band">{{ radioBand(w.channel) }}</span></span>
        <span class="text-right font-mono text-primary" data-test="radio-window-frames">{{ radioWindowFrames(w.frames) }}</span>
        <span class="col-start-2 col-end-4 flex flex-wrap items-baseline gap-x-2 text-muted">
          <span v-if="w.stop === 'frame_cap'" class="rounded-[var(--radius-sm)] bg-state-running/15 px-1.5 text-state-running" data-test="radio-window-badge">{{ RADIO_WINDOW_CUT_BADGE }}</span>
          <span v-if="w.stop === 'frame_cap'" class="text-state-running" data-test="radio-window-cut">{{ RADIO_WINDOW_CUT }}</span>
          <span v-if="w.own_frames" class="text-state-success" data-test="radio-window-own">{{ radioWindowOwn(w.own_frames) }}</span>
          <span v-if="w.kernel_dropped" class="text-state-running">{{ w.kernel_dropped }} {{ RADIO_WINDOW_MISSED }}</span>
          <span data-test="radio-window-note">{{ note(w) }}</span>
        </span>
      </li>
    </ul>
  </div>
</template>
