<script setup lang="ts">
import { computed, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import TrafficDeviceNames from "./TrafficDeviceNames.vue"
import TrafficGroupPlaces from "./TrafficGroupPlaces.vue"
import TrafficMap from "./TrafficMap.vue"
import TrafficPlaceGroups from "./TrafficPlaceGroups.vue"
import { useResponsiveLayout } from "../../composables/ui/useResponsiveLayout"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { formatBytes } from "../../utils/format/formatters"
import { formatRelativeTime } from "../../utils/format/time"
import { freshCount, groupPlaces, measureOf, placeDots, unlocatedCount } from "../../utils/netaudit/places"
import { routePoints } from "../../utils/netaudit/traffic"
import { toNetworkSettings } from "../../router/routes"
import {
  DEVICE_EMPTY_TITLE, DEVICE_MAP_FITS, DEVICE_MEASURE_NOTE, DEVICE_NO_PLACES, DEVICE_NOT_READ, DEVICE_SOURCE_LABELS, DEVICE_STATS, DEVICE_UNLOCATED, GROUP_DETAIL_CLEAR, GROUP_DETAIL_EMPTY_BODY, GROUP_DETAIL_EMPTY_TITLE,
  GROUPS_BY_LABEL, GROUPS_BY_LABELS, GROUPS_FILTER_LABEL, GROUPS_FILTER_ALL, GROUPS_FILTER_NEW, GROUPS_FILTER_UNLOCATED, GROUPS_NONE, GROUPS_PLACES, GROUPS_SEARCH, ROUTER_CONN_COUNT, TRAFFIC_MAP_NO_CITIES,
  TRAFFIC_MAP_NOTE, TRAFFIC_MAP_OPEN_TABLES, TRAFFIC_MAP_ROUTE_NOTE, TRAFFIC_MAP_TITLE,
} from "../../constants/netaudit"
import type { DeviceLookups, PlaceGroupBy, TrafficDevice } from "../../types/netaudit"
import type { ChoiceOption } from "../../types/ui"

// One device's page: who it is and where it was read, the numbers, a map of where it went, and its places grouped by company (or country). Picking a
// group, or its dot on the map, shows its places; a place's Route opens the route under it and draws it on the map. This computer and a device
// found in the router's table are the same page: only the source and the measure (bytes or connections) differ. `summary` and `more` are where this
// computer's own look puts its headline and its other detail.
const props = defineProps<{ device: TrafficDevice; lookups: DeviceLookups | null; lookupsEnabled: boolean; lookupsError?: string }>()
defineSlots<{ summary?(): unknown; more?(): unknown }>()
const { isMobile } = useResponsiveLayout()
const { traceOf } = useRouteTrace()

const by = ref<PlaceGroupBy>("company")
const filter = ref<"all" | "new" | "unlocated">("all")
const query = ref("")
const selected = ref<string | null>(null)
const routeAddress = ref<string | null>(null)
// What is open belongs to one device and one way of grouping.
watch(() => props.device.id, () => { by.value = "company"; filter.value = "all"; query.value = ""; selected.value = null; routeAddress.value = null })
watch([by, filter, query], () => { selected.value = null; routeAddress.value = null })

const rows = computed(() => props.device.rows)
const measure = computed(() => measureOf(rows.value))
const unlocated = computed(() => unlocatedCount(rows.value))
const fresh = computed(() => freshCount(rows.value))
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return rows.value.filter((r) => (filter.value === "all" || (filter.value === "new" ? r.fresh : !r.owner?.point)) && (!q || `${r.address} ${r.name ?? ""} ${r.owner?.name ?? ""}`.toLowerCase().includes(q)))
})
const groups = computed(() => groupPlaces(shown.value, by.value))
const group = computed(() => groups.value.find((g) => g.key === selected.value) ?? null)

const total = computed(() => (measure.value === "bytes" ? formatBytes(rows.value.reduce((n, r) => n + (r.bytes ?? 0), 0)) : String(rows.value.reduce((n, r) => n + r.count, 0))))
const stats = computed(() => [
  { id: "total", label: measure.value === "bytes" ? DEVICE_STATS.bytes : DEVICE_STATS.connections, value: total.value },
  { id: "places", label: DEVICE_STATS.places, value: String(rows.value.length) },
  { id: "companies", label: DEVICE_STATS.companies, value: String(groupPlaces(rows.value, "company").length) },
  { id: "countries", label: DEVICE_STATS.countries, value: String(groupPlaces(rows.value, "country").filter((g) => g.key).length) },
  ...(fresh.value > 0 ? [{ id: "fresh", label: DEVICE_STATS.fresh, value: String(fresh.value) }] : []),
])
const when = computed(() => (props.device.at ? formatRelativeTime(props.device.at) : ""))

const picked = computed(() => new Set([...(group.value?.rows.map((r) => r.address) ?? []), ...(routeAddress.value ? [routeAddress.value] : [])]))
const dots = computed(() => placeDots(shown.value, picked.value))
const route = computed(() => routePoints(routeAddress.value ? traceOf(routeAddress.value) : null))
const hasCities = computed(() => rows.value.some((r) => r.owner?.city || r.owner?.region))

function select(key: string) {
  selected.value = selected.value === key ? null : key
  routeAddress.value = null
}
// A dot opens the group of the heaviest place at it, and a second press closes it.
function pickDot(address: string) {
  const g = groups.value.find((x) => x.rows.some((r) => r.address === address))
  if (g) select(g.key)
}
const toggleRoute = (address: string) => { routeAddress.value = routeAddress.value === address ? null : address }

const byOptions: ChoiceOption[] = (["company", "country"] as const).map((value) => ({ value, label: GROUPS_BY_LABELS[value] }))
const filterOptions = computed<ChoiceOption[]>(() => [
  { value: "all", label: GROUPS_FILTER_ALL(rows.value.length) },
  ...(fresh.value > 0 ? [{ value: "new", label: GROUPS_FILTER_NEW(fresh.value) }] : []),
  ...(unlocated.value > 0 ? [{ value: "unlocated", label: GROUPS_FILTER_UNLOCATED(unlocated.value) }] : []),
])
</script>

<template>
  <div class="flex min-w-0 flex-col gap-4" data-test="device-pane">
    <header class="flex flex-wrap items-start justify-between gap-2">
      <div class="min-w-0">
        <h2 class="m-0 break-words text-[length:var(--text-heading)] font-semibold text-primary" data-test="device-name">{{ device.name }} <span class="font-mono text-[length:var(--text-small)] font-normal text-muted" data-test="device-address">{{ device.address }}</span></h2>
        <p v-if="when" class="m-0 text-[length:var(--text-small)] text-muted" data-test="device-when">{{ when }}</p>
      </div>
      <span class="rounded-full border border-strong px-2 py-0.5 font-mono text-[length:var(--text-micro)] text-muted" data-test="device-source">{{ DEVICE_SOURCE_LABELS[device.kind] }}</span>
    </header>

    <slot name="summary" />

    <EmptyState v-if="!rows.length" :title="DEVICE_EMPTY_TITLE" :body="device.at ? DEVICE_NO_PLACES : DEVICE_NOT_READ" data-test="device-empty" />
    <template v-else>
      <Panel>
        <dl class="m-0 grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fit,minmax(7rem,1fr))]" data-test="device-stats">
          <div v-for="s in stats" :key="s.id" class="flex flex-col gap-0.5">
            <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ s.label }}</dt>
            <dd class="m-0 font-mono text-[length:var(--text-numeral)] font-medium tabular-nums leading-[1.1] text-primary" :data-test="`stat-${s.id}`">{{ s.value }}</dd>
          </div>
        </dl>
      </Panel>

      <Panel :title="TRAFFIC_MAP_TITLE">
        <div class="flex flex-col gap-2">
          <TrafficMap v-if="dots.length" :dots="dots" :route="route" :measure="measure" @pick="pickDot" />
          <p v-else class="m-0 flex flex-wrap items-center gap-x-3 text-secondary" data-test="map-empty">
            <span>{{ hasCities ? GROUPS_NONE : TRAFFIC_MAP_NO_CITIES }}</span>
            <RouterLink v-if="!hasCities" :to="toNetworkSettings('data')" class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline sm:min-h-8" data-test="map-open-tables">{{ TRAFFIC_MAP_OPEN_TABLES }}</RouterLink>
          </p>
          <p v-if="dots.length" class="m-0 flex flex-wrap items-center gap-x-3 gap-y-1 text-[length:var(--text-small)] text-muted" data-test="map-notes">
            <span>{{ DEVICE_MEASURE_NOTE[measure] }}</span>
            <button v-if="unlocated && filter === 'all'" type="button" class="min-h-11 bg-transparent text-accent-info-text underline-offset-2 hover:underline sm:min-h-8" data-test="map-unlocated" @click="filter = 'unlocated'">{{ DEVICE_UNLOCATED(unlocated) }}</button>
            <span>{{ DEVICE_MAP_FITS }}</span>
            <span>{{ TRAFFIC_MAP_NOTE }}</span>
          </p>
          <p v-if="route.length > 1" class="m-0 text-[length:var(--text-small)] text-muted" data-test="map-route-note">{{ TRAFFIC_MAP_ROUTE_NOTE }}</p>
        </div>
      </Panel>

      <div class="grid min-w-0 gap-4 lg:grid-cols-2 lg:items-start">
        <Panel flush>
          <div class="flex flex-wrap items-center gap-2 border-b border-hairline/60 px-3 py-2">
            <SegmentedControl v-model="by" :options="byOptions" :label="GROUPS_BY_LABEL" variant="compact" data-test="group-by" />
            <SegmentedControl v-if="unlocated > 0 || fresh > 0" v-model="filter" :options="filterOptions" :label="GROUPS_FILTER_LABEL" variant="compact" data-test="group-filter" />
            <input
              v-model="query"
              type="search"
              autocomplete="off"
              :placeholder="GROUPS_SEARCH"
              :aria-label="GROUPS_SEARCH"
              class="h-11 min-w-0 flex-1 rounded-[var(--radius-md)] border border-control bg-canvas px-2 text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8"
              data-test="group-search"
            />
          </div>
          <p v-if="!groups.length" class="m-0 p-3 text-muted" data-test="groups-none">{{ GROUPS_NONE }}</p>
          <TrafficPlaceGroups v-else :groups="groups" :measure="measure" :selected="selected" :inline="isMobile" @select="select">
            <template #body="{ group: g }"><TrafficGroupPlaces :group="g" :route-address="routeAddress" :from-this-computer="device.kind === 'router'" @route="toggleRoute" /></template>
          </TrafficPlaceGroups>
        </Panel>

        <Panel v-if="!isMobile" data-test="group-detail">
          <div v-if="group" class="flex flex-col gap-2">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <h3 class="m-0 break-words text-[length:var(--text-body)] font-semibold text-primary" data-test="detail-title">{{ group.title }}</h3>
              <BaseButton variant="ghost" size="sm" data-test="detail-clear" @click="selected = null">{{ GROUP_DETAIL_CLEAR }}</BaseButton>
            </div>
            <p class="m-0 text-[length:var(--text-small)] text-muted">{{ GROUPS_PLACES(group.rows.length) }} · {{ measure === "bytes" ? formatBytes(group.bytes) : ROUTER_CONN_COUNT(group.count) }}</p>
            <TrafficGroupPlaces :group="group" :route-address="routeAddress" :from-this-computer="device.kind === 'router'" @route="toggleRoute" />
          </div>
          <div v-else class="flex flex-col gap-1" data-test="detail-empty">
            <h3 class="m-0 text-[length:var(--text-body)] font-semibold text-primary">{{ GROUP_DETAIL_EMPTY_TITLE }}</h3>
            <p class="m-0 text-muted">{{ GROUP_DETAIL_EMPTY_BODY }}</p>
          </div>
        </Panel>
      </div>
    </template>

    <Panel v-if="device.kind === 'router'">
      <TrafficDeviceNames :lookups="lookups" :enabled="lookupsEnabled" :known="Boolean(device.mac)" :error="lookupsError" />
    </Panel>

    <slot name="more" />
  </div>
</template>
