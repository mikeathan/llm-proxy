<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { RadioService } from "../../services/radio/radioService"
import { usePolling } from "../../composables/ui/usePolling"
import { errorMessage } from "../../utils/errors"
import { RADIO_TRACK_FAILED, RADIO_TRACK_NONE, RADIO_TRACK_NOTE_DISK, RADIO_TRACK_NOTE_MEMORY, RADIO_TRACK_TITLE, radioTrackSummary } from "../../constants/radio"
import type { RadioTrack } from "../../types/radio"

// Radio · one device's distance over this run: a band (the range it can be in) with a line through its middle, oldest on the left. A cycle the
// device was silent in is a gap, never a guessed point. Memory only on the server (ADR-0053), so a run that ends leaves nothing to draw.
const props = defineProps<{ id: string }>()
const REFRESH_MS = 5000
const W = 320
const H = 90
const PAD = 6
const MAX_M = 60

const track = ref<RadioTrack | null>(null)
const error = ref("")
const loaded = ref(false)

async function load(): Promise<void> {
  const id = props.id
  try {
    const t = await RadioService.track(id)
    if (id === props.id) { track.value = t; error.value = "" }
  } catch (err) {
    if (id !== props.id) return
    track.value = null
    // A 404 is the plain "nothing yet"; anything else is a failure the owner should see.
    error.value = /no track/i.test(errorMessage(err, "")) ? "" : errorMessage(err, RADIO_TRACK_FAILED)
  } finally {
    if (id === props.id) loaded.value = true
  }
}
watch(() => props.id, () => { track.value = null; loaded.value = false; void load() }, { immediate: true })
usePolling(() => { void load() }, REFRESH_MS)

// The time axis runs from the first point to now, so a stored track of a day and a run of an hour both fill the chart.
const first = computed(() => track.value?.points[0]?.at ?? 0)
const span = computed(() => Math.max(60, (track.value?.now ?? 0) - first.value))
const x = (at: number): number => PAD + ((at - first.value) / span.value) * (W - 2 * PAD)
const y = (metres: number): number => PAD + (1 - Math.log(Math.max(1, Math.min(MAX_M, metres))) / Math.log(MAX_M)) * (H - 2 * PAD)

// Runs as the server numbered them: a silent stretch ends one run and starts the next.
const runs = computed(() => {
  const out: NonNullable<typeof track.value>["points"][] = []
  for (const p of track.value?.points ?? []) {
    const last = out[p.run]
    if (last) last.push(p)
    else out[p.run] = [p]
  }
  return out.filter(Boolean)
})
const middle = (p: { near_m: number; far_m: number }): number => Math.sqrt(p.near_m * p.far_m)
const line = (run: typeof runs.value[number]): string => run.map((p) => `${x(p.at).toFixed(1)},${y(middle(p)).toFixed(1)}`).join(" ")
const band = (run: typeof runs.value[number]): string => [
  ...run.map((p) => `${x(p.at).toFixed(1)},${y(p.near_m).toFixed(1)}`),
  ...[...run].reverse().map((p) => `${x(p.at).toFixed(1)},${y(p.far_m).toFixed(1)}`),
].join(" ")
</script>

<template>
  <div class="flex flex-col gap-1" data-test="radio-track">
    <span class="text-[length:var(--text-small)] text-muted">{{ RADIO_TRACK_TITLE }}</span>
    <span v-if="error" class="text-state-error" data-test="radio-track-error">{{ error }}</span>
    <span v-else-if="loaded && !track" class="text-[length:var(--text-small)] text-muted" data-test="radio-track-none">{{ RADIO_TRACK_NONE }}</span>
    <template v-else-if="track">
      <svg :viewBox="`0 0 ${W} ${H}`" class="h-auto w-full max-w-md" role="img" :aria-label="radioTrackSummary(track.points.length, span)">
        <g v-for="(run, i) in runs" :key="i">
          <polygon v-if="run.length > 1" :points="band(run)" style="fill: rgb(var(--accent-brand)); opacity: 0.18" data-test="radio-track-band" />
          <polyline v-if="run.length > 1" :points="line(run)" fill="none" style="stroke: rgb(var(--accent-brand))" stroke-width="2" data-test="radio-track-line" />
          <template v-else>
            <circle v-for="p in run" :key="p.at" :cx="x(p.at)" :cy="y(middle(p))" r="2.5" style="fill: rgb(var(--accent-brand))" data-test="radio-track-line" />
          </template>
        </g>
      </svg>
      <span class="text-[length:var(--text-small)] text-muted" data-test="radio-track-summary">{{ radioTrackSummary(track.points.length, span) }}. {{ track.stored ? RADIO_TRACK_NOTE_DISK : RADIO_TRACK_NOTE_MEMORY }}</span>
    </template>
  </div>
</template>
