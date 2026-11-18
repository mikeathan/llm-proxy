<script setup lang="ts">
import { computed } from "vue"
import SelectInput from "../common/forms/SelectInput.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import {
  RADIO_COLOURS_PROVISIONAL, RADIO_TOGGLE_COLOURS, RADIO_TOGGLE_RANGE, RADIO_TOGGLE_SPOKE, RADIO_TOGGLE_SPOKE_HELP, RADIO_TOGGLE_USUAL, RADIO_VIEW_HELP,
  RADIO_VIEW_LABEL, RADIO_VIEW_ORDER, RADIO_VIEW_WORDS,
} from "../../constants/radio"
import { RADIO_FILTER_HIDE_BRIEF } from "../../constants/radioIdentity"
import type { RadioAroundView } from "../../types/radio"

// Radio · Around you: the drop-down that picks the view, and the switches of the radar. The choices are remembered per person.
const props = defineProps<{ mode: RadioAroundView; range: boolean; spoke: boolean; colours: boolean; usual: boolean; brief?: boolean }>()
const emit = defineEmits<{ (e: "update:mode", v: RadioAroundView): void; (e: "toggle", key: "range" | "spoke" | "colours" | "usual" | "brief", on: boolean): void }>()
const radarOnly = computed(() => props.mode !== "networks")
const options = RADIO_VIEW_ORDER.map((v) => ({ value: v, label: RADIO_VIEW_WORDS[v] }))
const help = computed(() => RADIO_VIEW_HELP[props.mode])
</script>

<template>
  <div class="flex flex-col gap-2" data-test="radio-around-controls">
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
      <label class="flex items-center gap-2 text-[length:var(--text-small)] text-secondary">{{ RADIO_VIEW_LABEL }}
        <SelectInput :model-value="mode" :options="options" :label="RADIO_VIEW_LABEL" class="max-sm:!h-11" data-test="radio-view-select" @update:model-value="emit('update:mode', $event as RadioAroundView)" />
      </label>
      <span class="text-[length:var(--text-small)] text-muted" data-test="radio-view-help">{{ help }}</span>
    </div>
    <div v-if="radarOnly" class="flex flex-wrap gap-x-5 gap-y-1" data-test="radio-toggles">
      <BaseToggle :model-value="!!brief" :label="RADIO_FILTER_HIDE_BRIEF" data-test="radio-toggle-brief" @update:model-value="emit('toggle', 'brief', $event)" />
      <BaseToggle :model-value="range" :label="RADIO_TOGGLE_RANGE" data-test="radio-toggle-range" @update:model-value="emit('toggle', 'range', $event)" />
      <BaseToggle :model-value="spoke" :label="RADIO_TOGGLE_SPOKE" :title="RADIO_TOGGLE_SPOKE_HELP" data-test="radio-toggle-spoke" @update:model-value="emit('toggle', 'spoke', $event)" />
      <BaseToggle :model-value="colours" :label="RADIO_TOGGLE_COLOURS" :title="RADIO_COLOURS_PROVISIONAL" data-test="radio-toggle-colours" @update:model-value="emit('toggle', 'colours', $event)" />
      <BaseToggle :model-value="usual" :label="RADIO_TOGGLE_USUAL" data-test="radio-toggle-usual" @update:model-value="emit('toggle', 'usual', $event)" />
    </div>
    <p v-if="colours && radarOnly" class="m-0 text-[length:var(--text-small)] text-faint" data-test="radio-colours-note">{{ RADIO_COLOURS_PROVISIONAL }}</p>
  </div>
</template>
