<script setup lang="ts">
import { computed } from "vue"
import RadioFilterChips from "./RadioFilterChips.vue"
import RadioKindPick from "./RadioKindPick.vue"
import RadioAroundRadar from "./RadioAroundRadar.vue"
import RadioDeviceList from "./RadioDeviceList.vue"
import RadioChannelBars from "./RadioChannelBars.vue"
import { RADIO_CHANNELS_HEAD, RADIO_CHANNELS_TITLE } from "../../constants/radio"
import { RADIO_RADAR_USUAL_SHOW_BRIEF, radioBriefHidden } from "../../constants/radioIdentity"
import { channelCounts } from "../../utils/radio/around"
import type { RadioAroundState } from "../../types/radio"

// Radio · view "Radar": chips, the kind drop-down, the radar and the one list side by side, a small channel strip under them. The chips, the
// kind and the channel narrow the radar and the list together; a device opens its own page.
const props = defineProps<{ around: RadioAroundState; routers: { name: string; channel: number }[]; now: number }>()
const a = computed(() => props.around)
const counts = computed(() => channelCounts(a.value.everyone.value))
</script>

<template>
  <div class="grid gap-4 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]" data-test="radio-view-radar">
    <RadioAroundRadar :around="around" />
    <div class="flex min-w-0 flex-col gap-3">
      <RadioFilterChips :devices="a.everyone.value" :model-value="a.filter.value" :today="a.today.value" @update:model-value="a.setFilter" />
      <RadioKindPick :kinds="a.kinds.value" :model-value="a.kind.value" @update:model-value="a.setKind" />
      <details v-if="counts.length" class="group" data-test="radio-channel-strip">
        <summary class="min-h-11 cursor-pointer list-none text-[length:var(--text-small)] text-secondary focus-visible:outline-none focus-visible:ring-2 sm:min-h-8">{{ RADIO_CHANNELS_TITLE }}<span class="ml-2 text-muted">{{ RADIO_CHANNELS_HEAD }}</span></summary>
        <RadioChannelBars :counts="counts" :routers="routers" :selected="a.channel.value" @pick="a.setChannel" />
      </details>
      <p v-if="a.briefHidden.value" class="m-0 flex flex-wrap items-baseline gap-x-3 text-[length:var(--text-small)] text-muted" data-test="radio-brief-hidden">{{ radioBriefHidden(a.briefHidden.value) }}
        <button type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" data-test="radio-brief-show" @click="a.setToggle('brief', false)">{{ RADIO_RADAR_USUAL_SHOW_BRIEF }}</button>
      </p>
      <RadioDeviceList :devices="a.shown.value" :now="now" :usual-hidden="a.usualHidden.value" :usual-on="a.toggles.value.usual" @open="a.open" @usual="a.setToggle('usual', $event)" />
    </div>
  </div>
</template>
