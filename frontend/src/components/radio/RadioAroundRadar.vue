<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import RadioRadarCanvas from "./RadioRadarCanvas.vue"
import { RADIO_CLASS_ORDER, RADIO_CLASS_WORDS, RADIO_COLOURS_PROVISIONAL, RADIO_RADAR_NO_DIRECTION } from "../../constants/radio"
import { RADIO_OWNERSHIP_WORDS, RADIO_ZOOM_IN, RADIO_ZOOM_OUT, RADIO_ZOOM_RESET, radioBeyondLine, radioZoomLine } from "../../constants/radioIdentity"
import { RADAR_EDGES, edgeToFit, zoomStep } from "../../utils/radio/radar"
import type { RadioOwnership } from "../../types/radio"
import type { RadioAroundState } from "../../types/radio"

// Radio · the radar of every view: the picture narrowed by the chips and the channel, with zoom buttons (the wheel and a pinch work too) and a
// legend of what the colours mean. Until the owner zooms, the edge fits the typical device drawn; "Show all" goes back to that. A dot opens the
// device's own page.
const props = defineProps<{ around: RadioAroundState }>()
const a = computed(() => props.around)
const chosenEdge = ref<number | null>(null)
const fitted = computed(() => edgeToFit(a.value.shown.value.map((d) => Math.sqrt(d.near_m * d.far_m))))
const edge = computed(() => chosenEdge.value ?? fitted.value)
const CLASS_DOT: Record<string, string> = { infrastructure: "bg-accent-info", personal: "bg-state-running", neighbour: "bg-text-muted", transient: "bg-state-queued", unknown: "bg-text-faint" }
const OWN_DOT: Record<RadioOwnership, string> = { mine: "bg-accent-brand", likely: "border-2 border-accent-brand bg-transparent", neighbour: "bg-text-muted", unknown: "bg-text-faint" }
const OWN_ORDER: RadioOwnership[] = ["mine", "likely", "neighbour", "unknown"]
const beyond = computed(() => a.value.shown.value.filter((d) => Math.sqrt(d.near_m * d.far_m) > edge.value).length)
const atMin = computed(() => edge.value <= (RADAR_EDGES[0] ?? 0))
const atMax = computed(() => edge.value >= (RADAR_EDGES[RADAR_EDGES.length - 1] ?? 0))
function zoom(next: number): void { chosenEdge.value = next }
</script>

<template>
  <div class="flex min-w-0 flex-col gap-2" data-test="radio-around-radar">
    <div class="relative">
      <RadioRadarCanvas :devices="a.shown.value" :selected="null" :ranges="a.toggles.value.range" :spoke="a.toggles.value.spoke" :colours="a.toggles.value.colours" :edge="edge" @pick="a.open" @zoom="zoom" />
      <div class="absolute right-1 top-1 flex flex-col gap-1" data-test="radio-zoom">
        <BaseButton variant="secondary" size="sm" icon="plus" icon-only :label="RADIO_ZOOM_IN" :disabled="atMin" class-name="max-sm:!h-11 max-sm:!w-11" data-test="radio-zoom-in" @click="zoom(zoomStep(edge, 'in'))" />
        <BaseButton variant="secondary" size="sm" icon="minus" icon-only :label="RADIO_ZOOM_OUT" :disabled="atMax" class-name="max-sm:!h-11 max-sm:!w-11" data-test="radio-zoom-out" @click="zoom(zoomStep(edge, 'out'))" />
      </div>
    </div>
    <p class="m-0 flex flex-wrap items-baseline gap-x-3 text-[length:var(--text-small)] text-muted" data-test="radio-zoom-line">
      {{ radioZoomLine(edge) }}<span v-if="beyond" class="text-secondary" data-test="radio-beyond">{{ radioBeyondLine(beyond) }}</span>
      <button v-if="chosenEdge !== null" type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" data-test="radio-zoom-reset" @click="chosenEdge = null">{{ RADIO_ZOOM_RESET }}</button>
    </p>
    <ul v-if="a.toggles.value.colours" class="m-0 flex list-none flex-wrap gap-x-3 gap-y-1 p-0 text-[length:var(--text-small)] text-muted" data-test="radio-legend" :title="RADIO_COLOURS_PROVISIONAL">
      <li v-for="c in RADIO_CLASS_ORDER" :key="c" class="inline-flex items-center gap-1.5"><span :class="['inline-block h-2 w-2 rounded-full', CLASS_DOT[c]]" aria-hidden="true" />{{ RADIO_CLASS_WORDS[c] }}</li>
    </ul>
    <ul v-else class="m-0 flex list-none flex-wrap gap-x-3 gap-y-1 p-0 text-[length:var(--text-small)] text-muted" data-test="radio-own-legend">
      <li v-for="o in OWN_ORDER" :key="o" class="inline-flex items-center gap-1.5"><span :class="['inline-block h-2.5 w-2.5 rounded-full', OWN_DOT[o]]" aria-hidden="true" />{{ RADIO_OWNERSHIP_WORDS[o] }}</li>
    </ul>
    <p class="m-0 text-[length:var(--text-small)] text-faint">{{ RADIO_RADAR_NO_DIRECTION }}</p>
  </div>
</template>
