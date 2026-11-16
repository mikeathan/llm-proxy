<script setup lang="ts">
import { computed } from "vue"
import { RADIO_CHIP_WORDS, RADIO_CLASS_ORDER, RADIO_FILTER_ALL, RADIO_FILTER_LABEL, RADIO_FILTER_MINE, RADIO_FILTER_UNUSUAL } from "../../constants/radio"
import { countFor } from "../../utils/radio/around"
import type { RadioAroundFilter, RadioLiveDevice } from "../../types/radio"

// Radio · the filter chips with their counts: everyone, yours, those unlike their usual, then each kind. The count is over every device the picture
// holds, so a chip never says 0 for something the list would show. A chip narrows both the radar and the list.
const props = defineProps<{ devices: RadioLiveDevice[]; modelValue: RadioAroundFilter }>()
const emit = defineEmits<{ (e: "update:modelValue", value: RadioAroundFilter): void }>()
const chips = computed(() => [
  { id: "all" as RadioAroundFilter, label: RADIO_FILTER_ALL },
  { id: "mine" as RadioAroundFilter, label: RADIO_FILTER_MINE },
  { id: "unusual" as RadioAroundFilter, label: RADIO_FILTER_UNUSUAL },
  ...RADIO_CLASS_ORDER.map((c) => ({ id: c as RadioAroundFilter, label: RADIO_CHIP_WORDS[c] })),
].map((c) => ({ ...c, n: countFor(props.devices, c.id) })).filter((c) => c.n > 0 || c.id === "all" || c.id === "mine"))
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
