<script setup lang="ts">
import { computed, onMounted, provide, ref, watch } from "vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import TrafficDevicePane from "./TrafficDevicePane.vue"
import TrafficDeviceRail from "./TrafficDeviceRail.vue"
import TrafficHeadline from "./TrafficHeadline.vue"
import TrafficReport from "./TrafficReport.vue"
import { useDeviceLookups } from "../../composables/netaudit/useDeviceLookups"
import { useDeviceNames } from "../../composables/netaudit/useDeviceNames"
import { useHostTraffic } from "../../composables/netaudit/useHostTraffic"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { usePolling } from "../../composables/ui/usePolling"
import { ROUTER_CONN_INJECTION, useRouterConnections } from "../../composables/netaudit/useRouterConnections"
import { placesFromHost, trafficDevices } from "../../utils/netaudit/places"
import { DEVICE_RAIL_EMPTY_BODY, DEVICE_RAIL_EMPTY_TITLE, ROUTE_POLL_MS } from "../../constants/netaudit"
import { IN_DASHBOARD_CARD } from "../../constants/dashboard"

// Network · Traffic, by device (the dashboard card): every device this computer knows (this computer from the last look it took through the
// privileged helper, and every device of the router's connection table) in one list, and the picked device's page beside it: a map, its places
// by company, and the route to any of them. It only reads: taking a look and setting up the router are on the Network page's Traffic tab.
// A card's Panels draw only their content, which drops the titles and actions of the several sections inside this one (what it was, where it went,
// the view switch). They are sections of the card here, not the card, so they draw as panels again.
provide(IN_DASHBOARD_CARD, false)
const { summary, loaded, loadError, load } = useHostTraffic()
// One reading of the router, shared by the device list that shows it and anything under it that asks.
const router = useRouterConnections()
provide(ROUTER_CONN_INJECTION, router)
const { devices: inventory, names, load: loadInventory } = useDeviceNames()
const lookups = useDeviceLookups()
// The route traces are kept by the server; one running fills in step by step, so they are read again while the card is on screen.
const routes = useRouteTrace()
onMounted(() => { void load(); void router.load(); void loadInventory(); void lookups.load(); void routes.load() })
usePolling(() => void routes.refresh(), ROUTE_POLL_MS)

const hostRows = computed(() => (summary.value ? placesFromHost(summary.value, names.value) : []))
const devices = computed(() =>
  trafficDevices({
    host: summary.value ? { interface: summary.value.interface, at: summary.value.at, rows: hostRows.value } : null,
    router: router.state.value?.last ?? null,
    inventory: inventory.value,
    lookups: lookups.devices.value,
  }),
)

// The device picked: this computer when it has looked, otherwise the busiest device of the router. A pick that is gone (a new read) falls back the same way.
const pickedId = ref<string | null>(null)
const device = computed(() => devices.value.find((d) => d.id === pickedId.value) ?? devices.value[0] ?? null)
watch(devices, (list) => { if (pickedId.value && !list.some((d) => d.id === pickedId.value)) pickedId.value = null })
// A new read of the router can bring devices the inventory and the watch had not met: read both again.
watch(() => router.state.value?.last?.at, () => { void loadInventory(); void lookups.load() })
const lookupsOf = computed(() => (device.value?.mac ? lookups.byMac(device.value.mac) ?? null : null))
</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-devices-block">
    <LoadingState v-if="!loaded" label="Reading the traffic look" :rows="3" data-test="traffic-loading" />
    <template v-else>
      <p v-if="loadError" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-load-error">{{ loadError }}</p>
      <div v-if="device" class="grid min-w-0 gap-4 lg:grid-cols-[17rem_minmax(0,1fr)] lg:items-start" data-test="traffic-devices">
        <TrafficDeviceRail :devices="devices" :selected-id="device.id" @select="pickedId = $event" />
        <TrafficDevicePane :device="device" :lookups="lookupsOf" :lookups-enabled="lookups.enabled.value" :lookups-error="lookups.error.value">
          <template v-if="device.kind === 'host' && summary" #summary><TrafficHeadline :traffic="summary" /></template>
          <template v-if="device.kind === 'host' && summary" #more><TrafficReport :traffic="summary" /></template>
        </TrafficDevicePane>
      </div>
      <EmptyState v-else-if="!loadError" :title="DEVICE_RAIL_EMPTY_TITLE" :body="DEVICE_RAIL_EMPTY_BODY" data-test="traffic-never" />
    </template>
  </div>
</template>
