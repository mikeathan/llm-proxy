<script setup lang="ts">
import { computed } from "vue"
import Icon from "../icons/Icon.vue"
import {
  RADIO_RADAR_NOT_HEARD, RADIO_MOVING_NOW, RADIO_RADAR_USUAL_ROW, RADIO_TREND_MARK, RADIO_TREND_WORDS, radioDbmLine, radioRangeLine, radioRouterShort,
} from "../../constants/radio"
import { RADIO_KIND_ICON, RADIO_OWNERSHIP_WORDS } from "../../constants/radioIdentity"
import { deviceFacts, deviceMakerGap, deviceMakerNote, deviceTags, deviceTitle, heardAgoWord, trailPoints } from "../../utils/radio/describe"
import { conclusionOf, ownershipOf } from "../../utils/radio/identity"
import type { RadioLiveDevice } from "../../types/radio"

// Radio · one row of the device list. It leads with what the device is called (your name for it, its name on your network, its make and model, its
// network, its maker and kind) and one line saying whose it probably is; then what it is, the distance, which way it is moving and when it was
// heard. The short id is a small handle at the end. A tap anywhere on the row opens the device's own page.
const props = defineProps<{ device: RadioLiveDevice; now: number }>()
const emit = defineEmits<{ (e: "open", id: string): void }>()
const TRAIL_W = 56
const TRAIL_H = 16
const facts = computed(() => [...deviceFacts(props.device), deviceMakerNote(props.device) || deviceMakerGap(props.device)].filter(Boolean))
const tags = computed(() => deviceTags(props.device))
const trail = computed(() => trailPoints(props.device.tail_m, TRAIL_W, TRAIL_H))
const heard = computed(() => (props.device.usual ? RADIO_RADAR_USUAL_ROW : props.device.heard ? heardAgoWord(props.device, props.now) : RADIO_RADAR_NOT_HEARD))
const own = computed(() => ownershipOf(props.device))
const icon = computed(() => RADIO_KIND_ICON[props.device.ident?.kind ?? "unknown"])
const OWN_CLASS = { mine: "border-accent-brand text-accent-brand", likely: "border-accent-brand/60 text-accent-brand", neighbour: "border-hairline text-muted", unknown: "border-hairline text-faint" } as const
</script>

<template>
  <div class="flex w-full items-start gap-2" data-test="radio-row" :data-own="own">
    <button type="button" class="flex min-h-11 min-w-0 flex-1 items-start gap-3 py-2 text-left hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2" :aria-label="`Open ${deviceTitle(device)}`" data-test="radio-radar-row-button" @click="emit('open', device.id)">
      <span class="mt-0.5 flex h-8 w-8 flex-none items-center justify-center rounded-[var(--radius-md)] border" :class="OWN_CLASS[own]" aria-hidden="true"><Icon :name="icon" size="sm" /></span>
      <span class="flex min-w-0 flex-1 flex-col gap-0.5">
        <span class="flex min-w-0 flex-wrap items-baseline gap-x-2">
          <span class="min-w-0 break-words" :class="own === 'mine' || own === 'likely' ? 'text-primary' : 'text-secondary'" data-test="radio-row-title">{{ deviceTitle(device) }}</span>
          <span class="text-[length:var(--text-small)]" :class="own === 'mine' || own === 'likely' ? 'text-accent-brand' : 'text-muted'" data-test="radio-row-own">{{ own === 'mine' ? RADIO_OWNERSHIP_WORDS.mine : conclusionOf(device) }}</span>
        </span>
        <span class="text-[length:var(--text-small)] text-muted" data-test="radio-row-facts">{{ facts.join(" · ") }}</span>
        <span v-if="tags.length" class="flex flex-wrap gap-1">
          <span v-for="t in tags" :key="t.text" class="rounded-full border px-2 text-[length:var(--text-small)]" :class="t.warn ? 'border-state-running text-state-running' : 'border-hairline text-muted'" data-test="radio-row-tag" :data-tag="t.test">{{ t.text }}</span>
        </span>
      </span>
      <span class="flex flex-none flex-col items-end gap-0.5 text-[length:var(--text-small)]">
        <span class="font-mono text-primary" data-test="radio-row-range">{{ radioRangeLine(device.near_m, device.far_m) }}</span>
        <span class="text-muted">{{ radioDbmLine(device.dbm) }}<template v-if="!device.usual && device.trend !== 'unknown'"> · {{ RADIO_TREND_MARK[device.trend] }} {{ RADIO_TREND_WORDS[device.trend] }}</template><template v-if="device.moving"> · <span class="text-accent-brand" data-test="radio-row-moving">{{ RADIO_MOVING_NOW }}</span></template></span>
        <span v-if="device.router" class="text-muted" data-test="radio-row-router">{{ radioRouterShort(device.router) }}</span>
        <svg v-if="trail" :viewBox="`0 0 ${TRAIL_W} ${TRAIL_H}`" class="h-4 w-14 text-accent-brand" role="img" aria-label="Where it was over the last readings" data-test="radio-row-trail"><polyline :points="trail" fill="none" stroke="currentColor" stroke-width="1.5" /></svg>
        <span class="text-faint" data-test="radio-row-heard">{{ heard }} · <span class="font-mono" data-test="radio-row-id">{{ device.id }}</span></span>
      </span>
      <span class="mt-2 flex-none text-muted" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>
    </button>
  </div>
</template>
