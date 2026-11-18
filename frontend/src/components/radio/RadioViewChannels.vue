<script setup lang="ts">
import { computed } from "vue"
import RadioAroundRadar from "./RadioAroundRadar.vue"
import RadioDeviceList from "./RadioDeviceList.vue"
import RadioChannelBars from "./RadioChannelBars.vue"
import {
  RADIO_CHANNELS_HEAD, RADIO_CHANNELS_TITLE, RADIO_CHANNEL_ALL, radioChannelCrowd, radioChannelFilterLine,
} from "../../constants/radio"
import { channelCounts } from "../../utils/radio/around"
import type { RadioAroundState } from "../../types/radio"

// Radio · view "By channel": how crowded each channel is, your own network marked on its channel, and the radar and list narrowed to the channel you
// pick. The headline answers "is my Wi-Fi on a crowded channel?".
const props = defineProps<{ around: RadioAroundState; routers: { name: string; channel: number }[]; now: number }>()
const a = computed(() => props.around)
const counts = computed(() => channelCounts(a.value.everyone.value))
const headline = computed(() => {
  const mine = props.routers.map((r) => ({ r, n: counts.value.find((c) => c.channel === r.channel)?.devices ?? 0 })).sort((x, y) => y.n - x.n)[0]
  const quietest = [...counts.value].sort((x, y) => x.devices - y.devices)[0]
  return mine && quietest ? radioChannelCrowd(mine.r.name, mine.r.channel, mine.n, quietest.channel) : ""
})
</script>

<template>
  <div class="flex flex-col gap-3" data-test="radio-view-channels">
    <p v-if="headline" class="m-0 text-primary" data-test="radio-channels-headline">{{ headline }}</p>
    <div class="grid gap-4 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]">
      <RadioAroundRadar :around="around" />
      <div class="flex min-w-0 flex-col gap-3">
        <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_CHANNELS_TITLE }} <span class="font-normal text-muted">{{ RADIO_CHANNELS_HEAD }}</span></h3>
        <RadioChannelBars :counts="counts" :routers="routers" :selected="a.channel.value" @pick="a.setChannel" />
        <p v-if="a.channel.value !== null" class="m-0 flex flex-wrap items-baseline gap-x-3 text-[length:var(--text-small)] text-secondary" data-test="radio-channel-note">
          {{ radioChannelFilterLine(a.channel.value) }}
          <button type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" data-test="radio-channel-clear" @click="a.setChannel(a.channel.value)">{{ RADIO_CHANNEL_ALL }}</button>
        </p>
        <RadioDeviceList :devices="a.shown.value" :now="now" :usual-hidden="a.usualHidden.value" :usual-on="a.toggles.value.usual" @open="a.open" @usual="a.setToggle('usual', $event)" />
      </div>
    </div>
  </div>
</template>
