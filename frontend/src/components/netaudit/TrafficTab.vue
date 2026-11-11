<script setup lang="ts">
import { computed, onMounted, provide, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import Panel from "../common/layout/Panel.vue"
import TrafficDevicePane from "./TrafficDevicePane.vue"
import TrafficDeviceRail from "./TrafficDeviceRail.vue"
import TrafficHeadline from "./TrafficHeadline.vue"
import TrafficReport from "./TrafficReport.vue"
import TrafficRouterSetup from "./TrafficRouterSetup.vue"
import TrafficTablesLine from "./TrafficTablesLine.vue"
import TrafficTraceCard from "./TrafficTraceCard.vue"
import { usePolling } from "../../composables/ui/usePolling"
import { useDeviceLookups } from "../../composables/netaudit/useDeviceLookups"
import { useDeviceNames } from "../../composables/netaudit/useDeviceNames"
import { useHostTraffic } from "../../composables/netaudit/useHostTraffic"
import { ROUTER_CONN_INJECTION, useRouterConnections } from "../../composables/netaudit/useRouterConnections"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { formatRelativeTime } from "../../utils/format/time"
import { placesFromHost, trafficDevices } from "../../utils/netaudit/places"
import {
  DEVICE_RAIL_EMPTY_BODY, DEVICE_RAIL_EMPTY_TITLE, ROUTER_PANEL_TITLE, TRAFFIC_BUTTON, TRAFFIC_OPEN_SETTINGS, TRAFFIC_PASSPHRASE_NOTE, TRAFFIC_RUNNING, TRAFFIC_SUBTITLE, TRAFFIC_TITLE,
  TRAFFIC_FOLLOW_INTERVAL_MS as FOLLOW_INTERVAL_MS, ROUTE_POLL_MS,
} from "../../constants/netaudit"
import type { NetworkTab } from "../../types/routes"

// Network · Traffic, by device: every device this page knows (this computer from the look it takes through the privileged helper, and every device of
// the router's connection table) in one list, and the picked device's page beside it: a map, its places by company, and the route to any of them.
// The button is there only when the helper can do it; when it cannot, the reason and the one page that fixes it are, and the last look stays readable.
// It makes no finding.
const emit = defineEmits<{ (e: "open-tab", tab: NetworkTab): void }>()
const { availability, summary, loaded, running, error, loadError, load, listen, follow } = useHostTraffic()
// The route traces are kept by the server; one running fills in step by step, so it is read every second while it runs, and not otherwise.
const routes = useRouteTrace()
// One reading of the router, shared by the card that reads it and the device list that shows it.
const router = useRouterConnections()
provide(ROUTER_CONN_INJECTION, router)
const { devices: inventory, names, load: loadInventory } = useDeviceNames()
const lookups = useDeviceLookups()
onMounted(() => { void load(); void routes.load(); void loadInventory(); void lookups.load() })
// A look started before a reload runs on the server for up to a minute; check every few seconds whether it has ended.
usePolling(follow, FOLLOW_INTERVAL_MS)
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
  <div class="flex flex-col gap-4" data-test="traffic-tab">
    <LoadingState v-if="!loaded" label="Reading the traffic look" :rows="3" data-test="traffic-loading" />
    <template v-else>
      <Panel :title="TRAFFIC_TITLE">
        <div class="flex flex-col gap-3">
          <p class="m-0 text-secondary">{{ TRAFFIC_SUBTITLE }}</p>
          <div v-if="availability.ok" class="flex flex-wrap items-center gap-3">
            <BaseButton :loading="running" :disabled="running" class-name="max-sm:!h-11" data-test="traffic-listen" @click="listen">{{ running ? TRAFFIC_RUNNING : TRAFFIC_BUTTON }}</BaseButton>
            <span class="text-[length:var(--text-small)] text-muted">{{ TRAFFIC_PASSPHRASE_NOTE }}</span>
          </div>
          <div v-else class="flex flex-wrap items-center gap-3" data-test="traffic-unavailable">
            <p class="m-0 min-w-0 break-words text-secondary">{{ availability.reason }}</p>
            <BaseButton v-if="availability.fixTab" variant="secondary" class-name="max-sm:!h-11" data-test="traffic-fix" @click="emit('open-tab', availability.fixTab)">{{ TRAFFIC_OPEN_SETTINGS }}</BaseButton>
          </div>
          <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-error">{{ error }}</p>
          <p v-if="loadError" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="traffic-load-error">{{ loadError }}</p>
          <p v-if="summary" class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-when">{{ summary.interface }} · {{ formatRelativeTime(summary.at) }}</p>
        </div>
      </Panel>

      <Panel :title="ROUTER_PANEL_TITLE">
        <TrafficRouterSetup />
      </Panel>

      <div v-if="device" class="grid min-w-0 gap-4 lg:grid-cols-[17rem_minmax(0,1fr)] lg:items-start" data-test="traffic-devices">
        <TrafficDeviceRail :devices="devices" :selected-id="device.id" @select="pickedId = $event" />
        <TrafficDevicePane :device="device" :lookups="lookupsOf" :lookups-enabled="lookups.enabled.value" :lookups-error="lookups.error.value">
          <template v-if="device.kind === 'host' && summary" #summary><TrafficHeadline :traffic="summary" /></template>
          <template v-if="device.kind === 'host' && summary" #more><TrafficReport :traffic="summary" /></template>
        </TrafficDevicePane>
      </div>
      <EmptyState v-else-if="!loadError" :title="DEVICE_RAIL_EMPTY_TITLE" :body="DEVICE_RAIL_EMPTY_BODY" data-test="traffic-never" />

      <TrafficTraceCard />

      <!-- The tables that name internet addresses are managed in Settings; here only whether they are on, and a link. -->
      <TrafficTablesLine />
    </template>
  </div>
</template>
