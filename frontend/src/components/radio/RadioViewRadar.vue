<script setup lang="ts">
import { computed } from "vue"
import RadioFilterChips from "./RadioFilterChips.vue"
import RadioAroundRadar from "./RadioAroundRadar.vue"
import RadioDeviceList from "./RadioDeviceList.vue"
import RadioChannelBars from "./RadioChannelBars.vue"
import { RADIO_CHANNELS_HEAD, RADIO_CHANNELS_TITLE } from "../../constants/radio"
import { channelCounts } from "../../utils/radio/around"
import type { RadioAroundState } from "../../types/radio"

// Radio · view "Radar": chips, the radar and the one list side by side, a small channel strip under the chips. The chips, the channel and the picked
// device narrow the radar and the list together.
const props = defineProps<{ around: RadioAroundState; routers: { name: string; channel: number }[]; now: number }>()
const a = computed(() => props.around)
const counts = computed(() => channelCounts(a.value.everyone.value))
</script>

<template>
  <div class="grid gap-4 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]" data-test="radio-view-radar">
    <RadioAroundRadar :around="around" />
    <div class="flex min-w-0 flex-col gap-3">
      <RadioFilterChips :devices="a.everyone.value" :model-value="a.filter.value" @update:model-value="a.setFilter" />
      <details v-if="counts.length" class="group" data-test="radio-channel-strip">
        <summary class="min-h-11 cursor-pointer list-none text-[length:var(--text-small)] text-secondary focus-visible:outline-none focus-visible:ring-2 sm:min-h-8">{{ RADIO_CHANNELS_TITLE }}<span class="ml-2 text-muted">{{ RADIO_CHANNELS_HEAD }}</span></summary>
        <RadioChannelBars :counts="counts" :routers="routers" :selected="a.channel.value" @pick="a.setChannel" />
      </details>
      <RadioDeviceList :devices="a.shown.value" :selected="a.selected.value" :now="now" :usual-hidden="a.usualHidden.value" :usual-on="a.toggles.value.usual" @pick="a.pick" @usual="a.setToggle('usual', $event)" />
    </div>
  </div>
</template>
