<script setup lang="ts">
import { computed } from "vue"
import { WORLD_LAND_PATH } from "../../constants/worldLand"
import { formatBytes } from "../../utils/format/formatters"
import { ROUTER_CONN_COUNT, TRAFFIC_MAP_LABEL } from "../../constants/netaudit"
import type { GeoPoint, MapDot, PlaceMeasure } from "../../types/netaudit"

// A world map of where the places of one device went: one dot per city the table of cities gives (bigger for more bytes, or more connections for the
// router's table), and, when a route trace is open, a line through its routers that have a city. The land is drawn once from a path built into the
// page (Natural Earth, public domain); nothing is fetched. The view fits the dots and the line, so a few cities are not lost in a whole world. A dot is
// a city, never a machine, and the words around the map say so.
const props = withDefaults(defineProps<{ dots: readonly MapDot[]; route?: readonly GeoPoint[]; measure?: PlaceMeasure }>(), { route: () => [], measure: "bytes" })
const emit = defineEmits<{ (e: "pick", address: string): void }>()

// The map shows latitudes from 90 north to 60 south (Antarctica is left out), in an equirectangular projection: one unit per degree.
const WIDTH = 360
const NORTH = 90
const SOUTH = -60
const HEIGHT = NORTH - SOUTH
const ASPECT = WIDTH / HEIGHT
// The least width of the fitted view, in degrees, and the room left around the dots (a share of their spread).
const MIN_SPAN = 60
const PADDING = 1.35
const MIN_RADIUS = 2
const MAX_RADIUS = 6
const x = (lon: number) => lon + WIDTH / 2
const y = (lat: number) => NORTH - lat

const view = computed(() => {
  const points = [...props.dots, ...props.route]
  if (!points.length) return { x: 0, y: 0, w: WIDTH, h: HEIGHT }
  const xs = points.map((p) => x(p.lon))
  const ys = points.map((p) => y(p.lat))
  const [minX, maxX, minY, maxY] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)]
  let w = Math.max((maxX - minX) * PADDING, MIN_SPAN)
  let h = w / ASPECT
  if ((maxY - minY) * PADDING > h) { h = (maxY - minY) * PADDING; w = h * ASPECT }
  if (w >= WIDTH || h >= HEIGHT) return { x: 0, y: 0, w: WIDTH, h: HEIGHT }
  const left = Math.min(Math.max((minX + maxX) / 2 - w / 2, 0), WIDTH - w)
  const top = Math.min(Math.max((minY + maxY) / 2 - h / 2, 0), HEIGHT - h)
  return { x: left, y: top, w, h }
})
// Marks keep the size they have on the whole world when the view is zoomed in.
const scale = computed(() => view.value.w / WIDTH)
const biggest = computed(() => Math.max(1, ...props.dots.map((d) => d.weight)))
const radius = (weight: number) => (MIN_RADIUS + (MAX_RADIUS - MIN_RADIUS) * Math.sqrt(weight / biggest.value)) * scale.value
const routeLine = computed(() => props.route.map((p) => `${x(p.lon)},${y(p.lat)}`).join(" "))
const anyPicked = computed(() => props.dots.some((d) => d.picked))
const dotClass = (d: MapDot) => (d.picked ? "fill-accent-brand" : "fill-accent-info")
const weightWords = (weight: number) => (props.measure === "bytes" ? formatBytes(weight) : ROUTER_CONN_COUNT(weight))
// The biggest dots are drawn first so a small one is never hidden under a big one.
const drawn = computed(() => [...props.dots].sort((a, b) => b.weight - a.weight))
</script>

<template>
  <figure class="m-0 flex flex-col gap-2" data-test="traffic-map">
    <svg :viewBox="`${view.x} ${view.y} ${view.w} ${view.h}`" class="mx-auto h-auto w-full overflow-hidden rounded-[var(--radius-md)] bg-surface" role="img" :aria-label="TRAFFIC_MAP_LABEL" data-test="map-svg">
      <path :d="WORLD_LAND_PATH" class="fill-surface-hover" />
      <polyline v-if="route.length > 1" :points="routeLine" fill="none" class="stroke-accent-brand" stroke-width="1.6" stroke-linejoin="round" stroke-dasharray="5 4" vector-effect="non-scaling-stroke" data-test="map-route" />
      <circle v-for="(p, i) in route" :key="`r${i}`" :cx="x(p.lon)" :cy="y(p.lat)" :r="1.2 * scale" class="fill-accent-brand" data-test="map-route-point" />
      <g v-for="d in drawn" :key="d.key" class="cursor-pointer" data-test="map-dot" @click="emit('pick', d.address)">
        <title>{{ d.label }} · {{ weightWords(d.weight) }}</title>
        <circle :cx="x(d.lon)" :cy="y(d.lat)" :r="radius(d.weight)" :class="dotClass(d)" :fill-opacity="d.picked || !anyPicked ? 0.75 : 0.25" />
      </g>
    </svg>
    <figcaption class="sr-only">
      <ul>
        <li v-for="d in drawn" :key="d.key">{{ d.label }}: {{ weightWords(d.weight) }}</li>
      </ul>
    </figcaption>
  </figure>
</template>
