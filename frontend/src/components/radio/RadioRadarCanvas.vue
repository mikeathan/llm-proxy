<script setup lang="ts">
import { RADIO_RADAR_LABEL, radioDotLabel } from "../../constants/radio"
import type { RadioClass, RadioLiveDevice } from "../../types/radio"

// Radio · the radar picture. Distance rings round the computer; one dot per device, placed by how strong its signal is on a log scale, with the
// range it could be in behind it and a fading tail of where it was in the last cycles. A dot glides to its new place and a device heard in the
// current cycle pulses; both stop for a person who asks for less motion. The direction is not measured: a dot keeps a stable place on its ring.
const props = defineProps<{ devices: RadioLiveDevice[]; selected: string | null }>()
const emit = defineEmits<{ (e: "pick", id: string): void }>()

const SIZE = 400
const CENTRE = SIZE / 2
const MAX_R = 176
const MAX_M = 60
const RING_METRES = [3, 10, 30]

const CLASS_COLOUR: Record<RadioClass, string> = {
  infrastructure: "rgb(var(--accent-info))", personal: "rgb(var(--state-running))", neighbour: "rgb(var(--text-muted))", transient: "rgb(var(--state-queued))", unknown: "rgb(var(--text-faint))",
}
function colour(d: RadioLiveDevice): string { return d.mine ? "rgb(var(--accent-brand))" : CLASS_COLOUR[d.class] }
function radius(metres: number): number { return Math.min(MAX_R, (MAX_R * Math.log(Math.max(1, metres))) / Math.log(MAX_M)) }
function point(angle: number, r: number): { x: number; y: number } {
  const a = ((angle - 90) * Math.PI) / 180
  return { x: CENTRE + r * Math.cos(a), y: CENTRE + r * Math.sin(a) }
}
const middle = (d: RadioLiveDevice): number => Math.sqrt(d.near_m * d.far_m)
function place(d: RadioLiveDevice) { return point(d.angle, radius(middle(d))) }
// Everything inside a device's group is drawn relative to its dot, so the group's transform is the only thing that moves.
function rel(d: RadioLiveDevice, metres: number) {
  const here = place(d)
  const there = point(d.angle, radius(metres))
  return { x: there.x - here.x, y: there.y - here.y }
}
function move(d: RadioLiveDevice): { transform: string } { const p = place(d); return { transform: `translate(${p.x}px, ${p.y}px)` } }
function tail(d: RadioLiveDevice) { return (d.tail_m ?? []).slice(0, -1).map((m, i, all) => ({ ...rel(d, m), a: 0.15 + (0.35 * (i + 1)) / all.length })) }
function showLabel(d: RadioLiveDevice): boolean { return d.id === props.selected || (d.mine && !d.usual) }
function labelSide(d: RadioLiveDevice): number { return place(d).x > CENTRE ? 1 : -1 }
</script>

<template>
  <svg :viewBox="`0 0 ${SIZE} ${SIZE}`" class="h-auto w-full max-w-[26rem] text-hairline" role="img" :aria-label="RADIO_RADAR_LABEL" data-test="radio-radar-svg">
    <circle v-for="m in RING_METRES" :key="m" :cx="CENTRE" :cy="CENTRE" :r="radius(m)" fill="none" stroke="currentColor" stroke-width="1" stroke-dasharray="3 4" />
    <text v-for="m in RING_METRES" :key="'t' + m" :x="CENTRE + 4" :y="CENTRE - radius(m) + 12" class="fill-current text-[10px] text-muted">{{ m }} m</text>
    <circle :cx="CENTRE" :cy="CENTRE" r="5" class="fill-current text-primary" />
    <g
      v-for="d in devices" :key="d.id" class="radio-dot cursor-pointer" :style="move(d)" data-test="radio-dot" :data-id="d.id" :data-ring="d.ring" :data-usual="d.usual ? 1 : 0"
      :opacity="d.usual ? 0.5 : d.heard ? 1 : 0.4" @click="emit('pick', d.id)"
    >
      <circle v-if="d.heard && !d.usual" r="7" fill="none" stroke-width="2" class="radio-pulse" :style="{ stroke: colour(d) }" data-test="radio-dot-pulse" />
      <circle v-for="(t, i) in tail(d)" :key="i" :cx="t.x" :cy="t.y" r="2" :style="{ fill: colour(d) }" :opacity="t.a" data-test="radio-dot-tail" />
      <circle v-if="d.unusual" r="9" fill="none" stroke-width="2" stroke-dasharray="2 2" style="stroke: rgb(var(--state-running))" data-test="radio-dot-unusual" />
      <line v-if="!d.usual" :x1="rel(d, d.near_m).x" :y1="rel(d, d.near_m).y" :x2="rel(d, d.far_m).x" :y2="rel(d, d.far_m).y" :style="{ stroke: colour(d) }" stroke-width="3" stroke-linecap="round" opacity="0.35" />
      <circle :r="d.mine ? 7 : 4.5" :style="{ fill: d.usual ? 'none' : colour(d), stroke: d.id === selected ? 'rgb(var(--text-primary))' : d.usual ? colour(d) : 'none' }" stroke-width="2" />
      <text v-if="showLabel(d)" :x="labelSide(d) * 11" y="4" :text-anchor="labelSide(d) > 0 ? 'start' : 'end'" class="fill-current text-[11px] text-primary" data-test="radio-dot-label">{{ radioDotLabel(d) }}</text>
    </g>
  </svg>
</template>

<style scoped>
.radio-dot { transition: transform 1.2s ease-in-out; }
.radio-pulse { transform-box: fill-box; transform-origin: center; animation: radio-ping 2.4s ease-out infinite; }
@keyframes radio-ping { from { transform: scale(1); opacity: 0.7; } to { transform: scale(2.6); opacity: 0; } }
@media (prefers-reduced-motion: reduce) {
  .radio-dot { transition: none; }
  .radio-pulse { animation: none; opacity: 0; }
}
</style>
