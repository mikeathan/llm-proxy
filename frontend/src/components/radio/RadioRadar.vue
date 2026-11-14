<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import RadioTrack from "./RadioTrack.vue"
import { useRadioLive } from "../../composables/radio/useRadioLive"
import { usePolling } from "../../composables/ui/usePolling"
import {
  RADIO_CLASS_WORDS, RADIO_RADAR_CALIBRATE, RADIO_RADAR_CALIBRATED, RADIO_RADAR_CLOSEST, RADIO_RADAR_EMPTY, RADIO_RADAR_INTRO, RADIO_RADAR_LABEL,
  RADIO_RADAR_LIVE_NOTE, RADIO_RADAR_MINE, RADIO_RADAR_NOT_HEARD, RADIO_RADAR_PROVISIONAL, RADIO_RADAR_TITLE, RADIO_RING_WORDS,
  RADIO_TREND_MARK, RADIO_TREND_WORDS, RADIO_RADAR_USUAL_LEGEND, RADIO_RADAR_USUAL_HIDE, RADIO_RADAR_USUAL_ROW, RADIO_RADAR_USUAL_SHOW,
  radioDotLabel, radioRadarSummary, radioRangeLine, radioShiftWord, radioUnusualSummary, radioUsualMore,
} from "../../constants/radio"
import type { RadioClass, RadioLiveDevice } from "../../types/radio"

// Radio · the radar. Distance rings round the computer, one dot per device heard now, placed by how strong its signal is (a log scale, so the
// rooms nearby are not squeezed). The faint line behind a dot is the range the distance can be in. The direction is NOT measured with one
// adapter, so a dot keeps a stable place on its ring and nothing more. The owner's devices carry their label; a neighbour is a short id.
const live = useRadioLive()
const REFRESH_MS = 5000
const SIZE = 400
const CENTRE = SIZE / 2
const MAX_R = 176
const MAX_M = 60
const RING_METRES = [3, 10, 30]

const selected = ref<string | null>(null)
const view = computed(() => live.view.value)
const everyone = computed(() => view.value?.devices ?? [])
const devices = computed(() => everyone.value.filter((d) => !d.usual))
const usual = computed(() => everyone.value.filter((d) => d.usual))
const unusualCount = computed(() => devices.value.filter((d) => d.unusual).length)
const showUsual = ref(false)
const mineCount = computed(() => devices.value.filter((d) => d.mine).length)
const byCloseness = (list: RadioLiveDevice[]) => [...list].sort((a, b) => Number(b.mine) - Number(a.mine) || a.near_m * a.far_m - b.near_m * b.far_m).slice(0, 12)
const closest = computed(() => byCloseness(devices.value))
const listed = computed(() => [...closest.value, ...usualRows.value])
const usualRows = computed(() => (showUsual.value ? byCloseness(usual.value) : []))
const chosen = computed(() => everyone.value.find((d) => d.id === selected.value) ?? null)
const calibrated = computed(() => !!view.value?.model.calibrated)

const CLASS_COLOUR: Record<RadioClass, string> = {
  infrastructure: "rgb(var(--accent-info))", personal: "rgb(var(--state-running))", neighbour: "rgb(var(--text-muted))", transient: "rgb(var(--state-queued))", unknown: "rgb(var(--text-faint))",
}

function colour(d: RadioLiveDevice): string { return d.mine ? "rgb(var(--accent-brand))" : CLASS_COLOUR[d.class] }
function radius(metres: number): number { return Math.min(MAX_R, (MAX_R * Math.log(Math.max(1, metres))) / Math.log(MAX_M)) }
function point(angle: number, r: number): { x: number; y: number } {
  const a = ((angle - 90) * Math.PI) / 180
  return { x: CENTRE + r * Math.cos(a), y: CENTRE + r * Math.sin(a) }
}
function dot(d: RadioLiveDevice) { return point(d.angle, radius(Math.sqrt(d.near_m * d.far_m))) }
function spanFrom(d: RadioLiveDevice) { return point(d.angle, radius(d.near_m)) }
function spanTo(d: RadioLiveDevice) { return point(d.angle, radius(d.far_m)) }
function pick(id: string): void { selected.value = selected.value === id ? null : id }
function showLabel(d: RadioLiveDevice): boolean { return d.id === selected.value || (d.mine && !d.usual) }
function labelSide(d: RadioLiveDevice): number { return dot(d).x > CENTRE ? 1 : -1 }

onMounted(() => { void live.load() })
usePolling(() => { void live.load() }, REFRESH_MS)
</script>

<template>
  <Panel :title="RADIO_RADAR_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-radar">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_RADAR_INTRO }}</p>
      <Callout v-if="live.error.value && !view" tone="error" title="Could not read"><span data-test="radio-radar-error">{{ live.error.value }}</span></Callout>
      <p v-else-if="view && !everyone.length" class="m-0 text-secondary" data-test="radio-radar-empty">{{ RADIO_RADAR_EMPTY }}</p>
      <template v-else-if="view">
        <p class="m-0 text-primary" data-test="radio-radar-summary">{{ radioRadarSummary(devices.length, mineCount) }}<template v-if="unusualCount"><span class="ml-2 text-state-running" data-test="radio-radar-unusual">{{ radioUnusualSummary(unusualCount) }}</span></template></p>
        <div class="grid gap-4 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]">
          <svg :viewBox="`0 0 ${SIZE} ${SIZE}`" class="h-auto w-full max-w-[26rem] text-hairline" role="img" :aria-label="RADIO_RADAR_LABEL" data-test="radio-radar-svg">
            <circle v-for="m in RING_METRES" :key="m" :cx="CENTRE" :cy="CENTRE" :r="radius(m)" fill="none" stroke="currentColor" stroke-width="1" stroke-dasharray="3 4" />
            <text v-for="m in RING_METRES" :key="'t' + m" :x="CENTRE + 4" :y="CENTRE - radius(m) + 12" class="fill-current text-[10px] text-muted">{{ m }} m</text>
            <circle :cx="CENTRE" :cy="CENTRE" r="5" class="fill-current text-primary" />
            <g v-for="d in everyone" :key="d.id" :data-test="'radio-dot'" :data-id="d.id" :data-ring="d.ring" :data-usual="d.usual ? 1 : 0" :opacity="d.usual ? 0.5 : d.heard ? 1 : 0.4" class="cursor-pointer" @click="pick(d.id)">
              <circle v-if="d.unusual" :cx="dot(d).x" :cy="dot(d).y" :r="d.mine ? 12 : 9" fill="none" stroke-width="2" stroke-dasharray="2 2" style="stroke: rgb(var(--state-running))" data-test="radio-dot-unusual" />
              <line v-if="!d.usual" :x1="spanFrom(d).x" :y1="spanFrom(d).y" :x2="spanTo(d).x" :y2="spanTo(d).y" :style="{ stroke: colour(d) }" stroke-width="3" stroke-linecap="round" opacity="0.35" />
              <circle :cx="dot(d).x" :cy="dot(d).y" :r="d.mine ? 7 : 4.5" :style="{ fill: d.usual ? 'none' : colour(d), stroke: d.id === selected ? 'rgb(var(--text-primary))' : d.usual ? colour(d) : 'none' }" stroke-width="2" />
              <text v-if="showLabel(d)" :x="dot(d).x + labelSide(d) * 11" :y="dot(d).y + 4" :text-anchor="labelSide(d) > 0 ? 'start' : 'end'" class="fill-current text-[11px] text-primary" data-test="radio-dot-label">{{ radioDotLabel(d) }}</text>
            </g>
          </svg>
          <div class="flex min-w-0 flex-col gap-2">
            <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_RADAR_CLOSEST }}</h3>
            <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-radar-list">
              <li v-for="d in listed" :key="d.id" data-test="radio-radar-row" :data-usual="d.usual ? 1 : 0">
                <button type="button" class="flex min-h-11 w-full items-baseline justify-between gap-3 py-1.5 text-left hover:text-primary focus-visible:outline-none focus-visible:ring-2" :aria-pressed="selected === d.id" @click="pick(d.id)">
                  <span class="flex min-w-0 flex-col">
                    <span class="truncate" :class="d.mine ? 'text-primary' : 'text-secondary'">{{ radioDotLabel(d) }}<span v-if="d.mine" class="ml-2 text-[length:var(--text-small)] text-accent-brand">{{ RADIO_RADAR_MINE }}</span></span>
                    <span class="text-[length:var(--text-small)] text-muted">{{ RADIO_CLASS_WORDS[d.class] }} · {{ RADIO_RING_WORDS[d.ring].toLowerCase() }}<template v-if="d.usual"> · {{ RADIO_RADAR_USUAL_ROW }}</template><template v-else-if="!d.heard"> · {{ RADIO_RADAR_NOT_HEARD }}</template></span>
                    <span v-if="d.unusual && d.usual_dbm" class="text-[length:var(--text-small)] text-state-running" data-test="radio-row-unusual">{{ radioShiftWord(d.shift_db ?? 0, d.usual_dbm) }}</span>
                  </span>
                  <span class="flex flex-none flex-col items-end text-[length:var(--text-small)]"><span class="font-mono text-primary">{{ radioRangeLine(d.near_m, d.far_m) }}</span><span v-if="!d.usual" class="text-muted">{{ RADIO_TREND_MARK[d.trend] }} {{ RADIO_TREND_WORDS[d.trend] }}</span></span>
                </button>
              </li>
            </ul>
            <p v-if="usual.length" class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[length:var(--text-small)] text-muted" data-test="radio-radar-usual">
              <span>{{ radioUsualMore(usual.length) }}</span>
              <button type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" :aria-pressed="showUsual" data-test="radio-usual-toggle" @click="showUsual = !showUsual">{{ showUsual ? RADIO_RADAR_USUAL_HIDE : RADIO_RADAR_USUAL_SHOW }}</button>
            </p>
            <div v-if="chosen" class="flex flex-col gap-2 rounded-[var(--radius-md)] bg-surface-raised px-3 py-2" data-test="radio-radar-chosen">
              <span class="text-primary">{{ radioDotLabel(chosen) }}: {{ chosen.usual ? "usually " : "" }}{{ chosen.dbm }} dBm on channel {{ chosen.channel }}, {{ radioRangeLine(chosen.near_m, chosen.far_m) }}<template v-if="!chosen.usual">, {{ RADIO_TREND_WORDS[chosen.trend] }}</template>.</span>
              <RadioTrack v-if="!chosen.usual" :id="chosen.id" />
              <button v-if="chosen.mine && !chosen.usual" type="button" class="min-h-11 self-start rounded-[var(--radius-md)] border border-hairline px-3 text-primary hover:bg-surface-hover disabled:opacity-60" :disabled="live.busy.value" data-test="radio-calibrate" @click="live.calibrate(chosen.id)">{{ RADIO_RADAR_CALIBRATE }}</button>
              <span v-if="live.error.value" class="text-state-error" data-test="radio-radar-action-error">{{ live.error.value }}</span>
            </div>
          </div>
        </div>
        <p v-if="usual.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-radar-legend">{{ RADIO_RADAR_USUAL_LEGEND }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-radar-model">{{ calibrated ? RADIO_RADAR_CALIBRATED : RADIO_RADAR_PROVISIONAL }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_RADAR_LIVE_NOTE }}</p>
      </template>
    </div>
  </Panel>
</template>
