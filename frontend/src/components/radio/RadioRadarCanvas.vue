<script setup lang="ts">
import { computed, ref } from "vue"
import { RADIO_RADAR_LABEL, radioRangeLine } from "../../constants/radio"
import { RADIO_RADAR_BEYOND } from "../../constants/radioIdentity"
import { deviceLabel } from "../../utils/radio/describe"
import { ownershipOf } from "../../utils/radio/identity"
import { RADAR_DEFAULT_EDGE, placeLabels, radiusOf, ringsFor, zoomStep } from "../../utils/radio/radar"
import type { RadioClass, RadioLiveDevice } from "../../types/radio"

// Radio · the radar picture. Distance rings round the computer; one dot per device, placed by how strong its signal is on a log scale from one metre
// to the edge, with the range it could be in behind it and a fading tail of where it was in the last cycles. The direction is not measured: a dot
// keeps a stable place on its ring. `edge` is the zoom (metres at the rim): the wheel and a two-finger pinch ask for the next one, and a device past
// the rim sits on it, faded. `solo` draws one device as a shaded ring from its nearest to its farthest distance, which is all one sensor can say.
// Dots are coloured by whose they are (yours, likely yours, not yours, unknown), or by their provisional class when `colours` is on.
const props = withDefaults(defineProps<{ devices: RadioLiveDevice[]; selected: string | null; ranges?: boolean; spoke?: boolean; colours?: boolean; edge?: number; solo?: boolean }>(), {
  ranges: false, spoke: false, colours: false, edge: RADAR_DEFAULT_EDGE, solo: false,
})
const emit = defineEmits<{ (e: "pick", id: string): void; (e: "zoom", edge: number): void }>()

const SIZE = 400
const CENTRE = SIZE / 2
const MAX_R = 176

const CLASS_COLOUR: Record<RadioClass, string> = {
  infrastructure: "rgb(var(--accent-info))", personal: "rgb(var(--state-running))", neighbour: "rgb(var(--text-muted))", transient: "rgb(var(--state-queued))", unknown: "rgb(var(--text-faint))",
}
const OWN_COLOUR = { mine: "rgb(var(--accent-brand))", likely: "rgb(var(--accent-brand))", neighbour: "rgb(var(--text-muted))", unknown: "rgb(var(--text-faint))" } as const
const rings = computed(() => ringsFor(props.edge))
function colour(d: RadioLiveDevice): string { return props.colours && !d.mine ? CLASS_COLOUR[d.class] : OWN_COLOUR[ownershipOf(d)] }
function hollow(d: RadioLiveDevice): boolean { return d.usual === true || (!props.colours && ownershipOf(d) === "likely") }
function radius(metres: number): number { return radiusOf(metres, props.edge, MAX_R).r }
function point(angle: number, r: number): { x: number; y: number } {
  const a = ((angle - 90) * Math.PI) / 180
  return { x: CENTRE + r * Math.cos(a), y: CENTRE + r * Math.sin(a) }
}
const middle = (d: RadioLiveDevice): number => Math.sqrt(d.near_m * d.far_m)
const beyond = (d: RadioLiveDevice): boolean => radiusOf(middle(d), props.edge, MAX_R).beyond
function place(d: RadioLiveDevice) { return point(d.angle, radius(middle(d))) }
// Everything inside a device's group is drawn relative to its dot, so the group's transform is the only thing that moves.
function rel(d: RadioLiveDevice, metres: number) {
  const here = place(d)
  const there = point(d.angle, radius(metres))
  return { x: there.x - here.x, y: there.y - here.y }
}
function move(d: RadioLiveDevice): { transform: string } { const p = place(d); return { transform: `translate(${p.x}px, ${p.y}px)` } }
function tail(d: RadioLiveDevice) { return (d.tail_m ?? []).slice(0, -1).map((m, i, all) => ({ ...rel(d, m), a: 0.15 + (0.35 * (i + 1)) / all.length })) }
function wantsLabel(d: RadioLiveDevice): boolean { return !props.solo && (props.ranges || d.id === props.selected || (d.mine && !d.usual)) } // a solo device is named by its page, not on the picture
// Labels that would overlap one already drawn are left out (the picked device first, then yours, then the closest), so a crowded picture stays readable.
const CHAR_W = 6
const LINE_H = 14
const labelled = computed(() => {
  const want = props.devices.filter(wantsLabel)
  const rank = (d: RadioLiveDevice): number => (d.id === props.selected ? 0 : d.mine ? 1 : 2)
  want.sort((a, b) => rank(a) - rank(b) || middle(a) - middle(b))
  return placeLabels(want, (d) => {
    const p = place(d)
    const w = labelText(d).length * CHAR_W
    return { x: labelSide(d) > 0 ? p.x + 12 : p.x - 12 - w, y: p.y - LINE_H / 2, w, h: LINE_H }
  })
})
function showLabel(d: RadioLiveDevice): boolean { return labelled.value.has(d) }
function labelText(d: RadioLiveDevice): string {
  const range = beyond(d) ? `${radioRangeLine(d.near_m, d.far_m)}, ${RADIO_RADAR_BEYOND}` : radioRangeLine(d.near_m, d.far_m)
  return d.id === props.selected || d.mine || props.solo ? `${deviceLabel(d)} · ${range}` : range
}
function labelSide(d: RadioLiveDevice): number { return place(d).x > CENTRE ? 1 : -1 }
function opacity(d: RadioLiveDevice): number { return (d.usual ? 0.5 : d.heard ? 1 : 0.4) * (beyond(d) ? 0.6 : 1) }

// The solo ring: from the nearest to the farthest the device could be, all the way round, because which way is not known.
const soloDevice = computed(() => (props.solo ? props.devices[0] ?? null : null))
const soloBand = computed(() => {
  const d = soloDevice.value
  if (!d) return null
  const r1 = radius(d.near_m)
  const r2 = radius(d.far_m)
  return { r: (r1 + r2) / 2, w: Math.max(2, r2 - r1) }
})

// Zoom by the wheel and by two fingers. A pinch asks for the next edge once the fingers have moved a quarter apart.
function wheel(e: WheelEvent): void {
  e.preventDefault()
  emit("zoom", zoomStep(props.edge, e.deltaY < 0 ? "in" : "out"))
}
const fingers = new Map<number, { x: number; y: number }>()
const pinchStart = ref(0)
function spread(): number {
  const [a, b] = [...fingers.values()]
  return a && b ? Math.hypot(a.x - b.x, a.y - b.y) : 0
}
function down(e: PointerEvent): void {
  fingers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (fingers.size === 2) pinchStart.value = spread()
}
function moved(e: PointerEvent): void {
  if (!fingers.has(e.pointerId)) return
  fingers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (fingers.size !== 2 || pinchStart.value === 0) return
  const ratio = spread() / pinchStart.value
  if (ratio > 1.25 || ratio < 0.8) {
    emit("zoom", zoomStep(props.edge, ratio > 1 ? "in" : "out"))
    pinchStart.value = spread()
  }
}
function up(e: PointerEvent): void {
  fingers.delete(e.pointerId)
  if (fingers.size < 2) pinchStart.value = 0
}
</script>

<template>
  <svg
    :viewBox="`0 0 ${SIZE} ${SIZE}`" class="h-auto w-full max-w-[26rem] touch-pan-y select-none text-hairline" role="img" :aria-label="RADIO_RADAR_LABEL" data-test="radio-radar-svg" :data-edge="edge"
    @wheel="wheel" @pointerdown="down" @pointermove="moved" @pointerup="up" @pointercancel="up" @pointerleave="up"
  >
    <circle v-for="m in rings" :key="m" :cx="CENTRE" :cy="CENTRE" :r="radius(m)" fill="none" stroke="currentColor" stroke-width="1" :stroke-dasharray="m === edge ? '' : '3 4'" data-test="radio-ring" />
    <text v-for="m in rings" :key="'t' + m" :x="CENTRE + 4" :y="CENTRE - radius(m) + 12" class="fill-current text-[10px] text-muted">{{ m }} m</text>
    <circle v-if="soloBand && soloDevice" :cx="CENTRE" :cy="CENTRE" :r="soloBand.r" fill="none" :style="{ stroke: colour(soloDevice) }" :stroke-width="soloBand.w" opacity="0.18" data-test="radio-solo-band" />
    <circle :cx="CENTRE" :cy="CENTRE" r="5" class="fill-current text-primary" />
    <g
      v-for="d in devices" :key="d.id" class="radio-dot cursor-pointer" :style="move(d)" data-test="radio-dot" :data-id="d.id" :data-ring="d.ring" :data-usual="d.usual ? 1 : 0" :data-beyond="beyond(d) ? 1 : 0"
      :opacity="opacity(d)" @click="emit('pick', d.id)"
    >
      <line v-if="spoke || solo" :x1="CENTRE - place(d).x" :y1="CENTRE - place(d).y" x2="0" y2="0" class="stroke-current text-muted" stroke-dasharray="2 3" opacity="0.6" data-test="radio-dot-spoke" />
      <circle v-if="d.heard && !d.usual" r="7" fill="none" stroke-width="2" class="radio-pulse" :style="{ stroke: colour(d) }" data-test="radio-dot-pulse" />
      <circle v-for="(t, i) in tail(d)" :key="i" :cx="t.x" :cy="t.y" r="2" :style="{ fill: colour(d) }" :opacity="t.a" data-test="radio-dot-tail" />
      <circle v-if="d.unusual" r="9" fill="none" stroke-width="2" stroke-dasharray="2 2" style="stroke: rgb(var(--state-running))" data-test="radio-dot-unusual" />
      <line v-if="!d.usual && !solo" :x1="rel(d, d.near_m).x" :y1="rel(d, d.near_m).y" :x2="rel(d, d.far_m).x" :y2="rel(d, d.far_m).y" :style="{ stroke: colour(d) }" stroke-width="3" stroke-linecap="round" opacity="0.35" />
      <circle :r="solo ? 8 : d.mine ? 7 : 4.5" :style="{ fill: hollow(d) ? 'rgb(var(--surface))' : colour(d), stroke: d.id === selected ? 'rgb(var(--text-primary))' : colour(d) }" stroke-width="2" />
      <text v-if="showLabel(d)" :x="labelSide(d) * 12" y="4" :text-anchor="labelSide(d) > 0 ? 'start' : 'end'" class="fill-current text-[11px] text-primary" data-test="radio-dot-label">{{ labelText(d) }}</text>
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
