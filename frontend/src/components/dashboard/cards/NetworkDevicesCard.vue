<script setup lang="ts">
import { computed, onMounted } from "vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useDevices } from "../../../composables/netaudit/useDevices"
import { useNetauditViewMode } from "../../../composables/netaudit/useNetauditViewMode"
import { toNetwork, toRadioDevice } from "../../../router/routes"
import { airWords } from "../../../utils/netaudit/air"
import { deviceAddress, deviceLabel, deviceSeen, deviceStatus, deviceSummary } from "../../../utils/netaudit/devices"
import { DEVICES_EMPTY, DEVICE_GROUP_TITLES } from "../../../constants/netaudit"
import type { DeviceStatus, NetworkDevice } from "../../../types/netaudit"

// Dashboard card · Devices: who is on the network, one row each, from the core's own list (which already carries the presence check and what Radio
// heard of the device, ADR-0061), in the words the Network page uses. It only reads: naming, approving and forgetting a device are on the Network
// page's Devices tab. Small is the count; medium the devices that need a look and those online; large every device.
const SHOWN = { xs: 0, sm: 6, lg: 40 } as const
const RANK: Record<DeviceStatus, number> = { attention: 0, online: 1, offline: 2 }
const TAG = { attention: "warning", online: "running", offline: "neutral" } as const
const size = useCardSize()
const { inventory, loadError, load } = useDevices()
const { advanced } = useNetauditViewMode()
const devices = computed(() => inventory.value?.devices ?? [])
const rows = computed(() => {
  const now = Date.now()
  return devices.value
    .map((device) => ({ device, status: deviceStatus(device, now) }))
    .sort((a, b) => RANK[a.status] - RANK[b.status] || Date.parse(b.device.last_seen) - Date.parse(a.device.last_seen))
    .slice(0, SHOWN[size.value])
})
const airOf = (device: NetworkDevice) => airWords(device.air, inventory.value?.air?.state, advanced.value, inventory.value?.air?.last_cycle)
// The page id is made by the core and checked here before it becomes a link, as the Network page's row does.
const radioPage = (device: NetworkDevice): string => (/^[0-9a-f]{8}$/.test(device.air?.page ?? "") ? device.air!.page! : "")
onMounted(load)
</script>

<template>
  <div data-test="network-devices-card">
    <ErrorState v-if="loadError && !inventory" title="The devices could not be read" :cause="loadError" next="Check that the server is running." />
    <LoadingState v-else-if="!inventory" label="Reading the devices" :rows="3" />
    <EmptyState v-else-if="!devices.length" title="No devices yet" :body="DEVICES_EMPTY" />
    <template v-else>
      <p class="m-0 text-primary" data-test="devices-card-summary">{{ deviceSummary(devices) }}</p>
      <ul v-if="rows.length" class="m-0 mt-3 flex list-none flex-col p-0" aria-label="Devices">
        <li v-for="r in rows" :key="r.device.mac" class="flex flex-col gap-0.5 border-b border-hairline/60 py-2 last:border-b-0" :data-test="`devices-card-${r.device.mac}`">
          <span class="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span class="min-w-0 break-words text-primary">{{ deviceLabel(r.device) }}</span>
            <StatusTag :state="TAG[r.status]" :label="DEVICE_GROUP_TITLES[r.status]" />
          </span>
          <span class="break-all text-[length:var(--text-small)] text-muted">{{ deviceAddress(r.device, advanced) }} · {{ deviceSeen(r.device) }}</span>
          <span v-if="airOf(r.device)" class="text-[length:var(--text-small)] text-secondary" data-test="devices-card-air">
            <RouterLink v-if="radioPage(r.device)" :to="toRadioDevice(radioPage(r.device))" class="text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2">{{ airOf(r.device)!.heard }}</RouterLink>
            <template v-else>{{ airOf(r.device)!.heard }}</template>
            <template v-if="airOf(r.device)!.rest">, {{ airOf(r.device)!.rest }}</template>
          </span>
        </li>
      </ul>
      <p v-if="size !== 'xs' && devices.length > rows.length" class="m-0 mt-2 text-[length:var(--text-small)] text-muted" data-test="devices-card-more">
        {{ devices.length - rows.length }} more. <RouterLink :to="toNetwork('devices')" class="text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2">Open the Devices tab</RouterLink>
      </p>
    </template>
  </div>
</template>
