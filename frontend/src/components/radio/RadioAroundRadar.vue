<script setup lang="ts">
import { computed } from "vue"
import RadioRadarCanvas from "./RadioRadarCanvas.vue"
import { RADIO_CLASS_ORDER, RADIO_CLASS_WORDS, RADIO_COLOURS_PROVISIONAL, RADIO_FOCUS_ALL, RADIO_RADAR_NO_DIRECTION, radioFocusLine } from "../../constants/radio"
import { deviceTitle } from "../../utils/radio/describe"
import type { RadioAroundState } from "../../types/radio"

// Radio · the radar of every view: the picture narrowed by the chips, the channel and the focus, with a note when only one device is drawn.
const props = defineProps<{ around: RadioAroundState }>()
const a = computed(() => props.around)
const focus = computed(() => (a.value.chosen.value && a.value.focused.value ? a.value.chosen.value : null))
const CLASS_DOT: Record<string, string> = { infrastructure: "bg-accent-info", personal: "bg-state-running", neighbour: "bg-text-muted", transient: "bg-state-queued", unknown: "bg-text-faint" }
</script>

<template>
  <div class="flex min-w-0 flex-col gap-2" data-test="radio-around-radar">
    <RadioRadarCanvas :devices="a.drawn.value" :selected="a.selected.value" :ranges="a.toggles.value.range" :spoke="a.toggles.value.spoke" :colours="a.toggles.value.colours" @pick="a.pick" />
    <p v-if="focus" class="m-0 flex flex-wrap items-baseline gap-x-3 text-[length:var(--text-small)] text-secondary" data-test="radio-focus-note">
      {{ radioFocusLine(deviceTitle(focus)) }}
      <button type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" data-test="radio-focus-all" @click="a.focused.value = false">{{ RADIO_FOCUS_ALL }}</button>
    </p>
    <ul v-if="a.toggles.value.colours" class="m-0 flex list-none flex-wrap gap-x-3 gap-y-1 p-0 text-[length:var(--text-small)] text-muted" data-test="radio-legend" :title="RADIO_COLOURS_PROVISIONAL">
      <li v-for="c in RADIO_CLASS_ORDER" :key="c" class="inline-flex items-center gap-1.5"><span :class="['inline-block h-2 w-2 rounded-full', CLASS_DOT[c]]" aria-hidden="true" />{{ RADIO_CLASS_WORDS[c] }}</li>
    </ul>
    <p class="m-0 text-[length:var(--text-small)] text-faint">{{ RADIO_RADAR_NO_DIRECTION }}</p>
  </div>
</template>
