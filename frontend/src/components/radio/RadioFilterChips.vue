<script setup lang="ts">
import { computed } from "vue"
import { RADIO_CHIP_WORDS, RADIO_CLASS_ORDER, RADIO_FILTER_ALL, RADIO_FILTER_LABEL, RADIO_FILTER_MINE, RADIO_FILTER_UNUSUAL } from "../../constants/radio"
import { RADIO_FILTER_LIKELY, RADIO_FILTER_NEW } from "../../constants/radioIdentity"
import { countFor } from "../../utils/radio/around"
import type { RadioAroundFilter, RadioLiveDevice } from "../../types/radio"

// Radio · the filter chips with their counts: everyone, yours, likely yours (the evidence says so), new today, those unlike their usual, then
// each class. What a device is (router, phone, speaker…) is the kind drop-down beside them (RadioKindPick), not a chip. The count is over every
// device the picture holds, so a chip never says 0 for something the list would show. A chip narrows both
// the radar and the list; a chip with nothing in it is not shown, except All and Yours.
const props = defineProps<{ devices: RadioLiveDevice[]; modelValue: RadioAroundFilter; today?: string }>()
const emit = defineEmits<{ (e: "update:modelValue", value: RadioAroundFilter): void }>()
const chips = computed(() => [
  { id: "all" as RadioAroundFilter, label: RADIO_FILTER_ALL },
  { id: "mine" as RadioAroundFilter, label: RADIO_FILTER_MINE },
  { id: "likely" as RadioAroundFilter, label: RADIO_FILTER_LIKELY },
  { id: "new" as RadioAroundFilter, label: RADIO_FILTER_NEW },
  { id: "unusual" as RadioAroundFilter, label: RADIO_FILTER_UNUSUAL },
  ...RADIO_CLASS_ORDER.map((c) => ({ id: c as RadioAroundFilter, label: RADIO_CHIP_WORDS[c] })),
].map((c) => ({ ...c, n: countFor(props.devices, c.id, props.today ?? "") })).filter((c) => c.n > 0 || c.id === "all" || c.id === "mine" || c.id === props.modelValue))
</script>

<template>
  <div role="group" :aria-label="RADIO_FILTER_LABEL" class="flex flex-wrap gap-1.5" data-test="radio-filter">
    <button
      v-for="c in chips" :key="c.id" type="button" :aria-pressed="modelValue === c.id" :data-filter="c.id" data-test="radio-chip"
      class="inline-flex min-h-11 items-center gap-1.5 rounded-[var(--radius-md)] border px-3 text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8"
      :class="modelValue === c.id ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover'"
      @click="emit('update:modelValue', c.id)"
    >{{ c.label }}<span class="font-mono text-muted">{{ c.n }}</span></button>
  </div>
</template>
