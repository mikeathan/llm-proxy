<script setup lang="ts">
import { computed } from "vue"
import { RADIO_CHANNEL_MINE, radioBand, radioChannelWord } from "../../constants/radio"
import type { RadioChannelCount } from "../../types/radio"

// Radio · the channels and how many devices are heard on each, as bars you can pick. A channel one of your own networks is on says so. Picking one
// narrows the radar and the list to the devices heard on it; picking it again clears it.
const props = defineProps<{ counts: RadioChannelCount[]; routers: { name: string; channel: number }[]; selected: number | null }>()
const emit = defineEmits<{ (e: "pick", channel: number): void }>()
const most = computed(() => Math.max(1, ...props.counts.map((c) => c.devices)))
const mineOn = (channel: number): string[] => props.routers.filter((r) => r.channel === channel).map((r) => r.name)
</script>

<template>
  <ul class="m-0 flex list-none flex-col gap-0.5 p-0" data-test="radio-channel-bars">
    <li v-for="c in counts" :key="c.channel">
      <button type="button" class="grid min-h-11 w-full grid-cols-[minmax(0,8rem)_minmax(0,1fr)_2rem] items-center gap-x-3 rounded-[var(--radius-md)] px-1 text-left text-[length:var(--text-small)] hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" :class="selected === c.channel ? 'bg-surface-active' : ''" :aria-pressed="selected === c.channel" :data-channel="c.channel" data-test="radio-channel-bar" @click="emit('pick', c.channel)">
        <span class="min-w-0 truncate text-secondary">{{ radioChannelWord(c.channel) }} <span class="text-muted">{{ radioBand(c.channel) }}</span><span v-if="mineOn(c.channel).length" class="ml-1 text-state-success" data-test="radio-channel-mine">{{ RADIO_CHANNEL_MINE }}</span></span>
        <span class="h-2 overflow-hidden rounded-full bg-hairline"><span class="block h-full" :class="mineOn(c.channel).length ? 'bg-state-success' : 'bg-accent-brand'" :style="{ width: Math.round((100 * c.devices) / most) + '%' }" /></span>
        <span class="text-right font-mono text-primary">{{ c.devices }}</span>
      </button>
    </li>
  </ul>
</template>
