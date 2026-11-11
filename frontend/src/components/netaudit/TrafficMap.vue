<script setup lang="ts">
import { computed } from "vue"
import { WORLD_LAND_PATH } from "../../constants/worldLand"
import { formatBytes } from "../../utils/format/formatters"
import { TRAFFIC_MAP_LABEL } from "../../constants/netaudit"
import type { GeoPoint, MapDot } from "../../types/netaudit"

// A world map of where the internet traffic of one look went: one dot per city the table of cities gives (bigger for more traffic), and, when a
// route trace is open, a line through its routers that have a city. The land is drawn once from a path built into the page (Natural Earth, public
// domain); nothing is fetched. A dot is a city, never a machine, and the words under the map say so.
const props = defineProps<{ dots: readonly MapDot[]; route?: readonly GeoPoint[] }>()
const emit = defineEmits<{ (e: "pick", address: string): void }>()

// The map shows latitudes from 90 north to 60 south (Antarctica is left out), in an equirectangular projection: one unit per degree.
const WIDTH = 360
const NORTH = 90
const SOUTH = -60
const HEIGHT = NORTH - SOUTH
const MIN_RADIUS = 2
const MAX_RADIUS = 6
const x = (lon: number) => lon + WIDTH / 2
const y = (lat: number) => NORTH - lat
const biggest = computed(() => Math.max(1, ...props.dots.map((d) => d.bytes)))
const radius = (bytes: number) => MIN_RADIUS + (MAX_RADIUS - MIN_RADIUS) * Math.sqrt(bytes / biggest.value)
const routeLine = computed(() => (props.route ?? []).map((p) => `${x(p.lon)},${y(p.lat)}`).join(" "))
// The biggest dots are drawn first so a small one is never hidden under a big one.
const drawn = computed(() => [...props.dots].sort((a, b) => b.bytes - a.bytes))
</script>

<template>
  <figure class="m-0 flex flex-col gap-2" data-test="traffic-map">
    <svg :viewBox="`0 0 ${WIDTH} ${HEIGHT}`" class="mx-auto h-auto w-full max-w-3xl overflow-hidden" role="img" :aria-label="TRAFFIC_MAP_LABEL">
      <path :d="WORLD_LAND_PATH" class="fill-surface-hover" />
      <polyline v-if="route && route.length > 1" :points="routeLine" fill="none" class="stroke-accent-brand" stroke-width="0.8" stroke-linejoin="round" stroke-dasharray="2 1.5" data-test="map-route" />
      <circle v-for="(p, i) in route ?? []" :key="`r${i}`" :cx="x(p.lon)" :cy="y(p.lat)" r="1.2" class="fill-accent-brand" data-test="map-route-point" />
      <g v-for="d in drawn" :key="d.key" class="cursor-pointer" data-test="map-dot" @click="emit('pick', d.address)">
        <title>{{ d.label }} · {{ formatBytes(d.bytes) }}</title>
        <circle :cx="x(d.lon)" :cy="y(d.lat)" :r="radius(d.bytes)" :class="d.picked ? 'fill-accent-brand' : 'fill-accent-info'" fill-opacity="0.75" />
      </g>
    </svg>
    <figcaption class="sr-only">
      <ul>
        <li v-for="d in drawn" :key="d.key">{{ d.label }}: {{ formatBytes(d.bytes) }}</li>
      </ul>
    </figcaption>
  </figure>
</template>
