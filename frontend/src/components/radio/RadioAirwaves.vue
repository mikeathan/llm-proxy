<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import { useRadioAirwaves } from "../../composables/radio/useRadioAirwaves"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  RADIO_AIR_CAP_NOTE, RADIO_AIR_CHANNELS, RADIO_AIR_CLASSES, RADIO_AIR_CLASSES_HELP, RADIO_AIR_EMPTY, RADIO_RETRIES_NOT_REPORTED, RADIO_AIR_INTRO, RADIO_AIR_TITLE,
  RADIO_CLASS_WORDS, radioAirBusiest, radioAirClassLine, radioAirFramesLine, radioAirRouter, radioChannelWord, radioBand, radioNeighNearUnknown,
} from "../../constants/radio"

// Radio · Airwaves: how crowded each channel is and which kinds of device spend the air on retries. Counts only; no neighbour is listed.
// The channel bars are always shown (the part that answers "where is it crowded"); Advanced adds retries by kind.
const a = useRadioAirwaves()
const { advanced } = useNetauditViewMode()
const view = computed(() => a.view.value)
const busiest = computed(() => [...(view.value?.channels ?? [])].sort((x, y) => y.devices - x.devices)[0])
const most = computed(() => Math.max(1, ...(view.value?.channels ?? []).map((c) => c.devices)))
const retriesSilent = computed(() => {
  const c = view.value?.classes ?? []
  return c.reduce((n, x) => n + x.frames, 0) >= 1000 && c.every((x) => x.retry_percent === 0)
})
const empty = computed(() => !!view.value && !view.value.channels.length && !view.value.classes.length)

onMounted(() => { void a.load() })
</script>

<template>
  <Panel :title="RADIO_AIR_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-airwaves">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_AIR_INTRO }}</p>
      <Callout v-if="a.error.value && !view" tone="error" title="Could not read"><span data-test="radio-air-error">{{ a.error.value }}</span></Callout>
      <p v-else-if="empty" class="m-0 text-secondary" data-test="radio-air-empty">{{ RADIO_AIR_EMPTY }}</p>
      <template v-else-if="view">
        <ul class="m-0 flex list-none flex-col gap-1 p-0 text-primary">
          <li v-if="busiest" data-test="radio-air-busiest">{{ radioAirBusiest(busiest.channel, busiest.devices) }}</li>
          <li v-for="r in view.routers" :key="r.name + r.channel" data-test="radio-air-router">{{ radioAirRouter(r.name, r.channel) }}</li>
          <li v-if="view.near_unknown" class="text-state-running" data-test="radio-air-near">{{ radioNeighNearUnknown(view.near_unknown) }}</li>
        </ul>
        <div class="flex flex-col gap-1">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_AIR_CHANNELS }}</h3>
          <ul class="m-0 flex list-none flex-col gap-1 p-0">
            <li v-for="c in view.channels" :key="c.channel" class="grid grid-cols-[7.5rem_minmax(0,1fr)_2rem] items-center gap-x-3 text-[length:var(--text-small)]" data-test="radio-air-channel" :data-channel="c.channel">
              <span class="text-secondary">{{ radioChannelWord(c.channel) }} <span class="text-muted">{{ radioBand(c.channel) }}</span></span>
              <span class="h-2 overflow-hidden rounded-full bg-hairline"><span class="block h-full bg-accent-brand" data-test="radio-air-channel-bar" :style="{ width: Math.round((100 * c.devices) / most) + '%' }" /></span>
              <span class="text-right font-mono text-primary">{{ c.devices }}</span>
            </li>
          </ul>
        </div>
        <template v-if="advanced">
          <div class="flex flex-col gap-1">
            <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_AIR_CLASSES }}</h3>
            <p v-if="retriesSilent" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="radio-air-retries-silent">{{ RADIO_RETRIES_NOT_REPORTED }}</p>
            <p v-else class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_AIR_CLASSES_HELP }}</p>
            <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
              <li v-for="c in view.classes" :key="c.class" class="flex flex-wrap items-baseline justify-between gap-x-3 py-1.5 text-[length:var(--text-small)]" data-test="radio-air-class">
                <span class="text-primary">{{ RADIO_CLASS_WORDS[c.class] }}</span>
                <span class="text-secondary">{{ retriesSilent ? radioAirFramesLine(c.devices, c.frames) : radioAirClassLine(c.devices, c.frames, c.retry_percent) }}</span>
              </li>
            </ul>
          </div>
          <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-air-cap-note">{{ RADIO_AIR_CAP_NOTE }}</p>
        </template>
      </template>
    </div>
  </Panel>
</template>
