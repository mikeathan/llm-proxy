<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import DeviceProbe from "./DeviceProbe.vue"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_COLUMNS, TRAFFIC_DETAIL_ADDRESS, TRAFFIC_DETAIL_CLOSE, TRAFFIC_DETAIL_NO_FLOWS, TRAFFIC_DETAIL_NOT_A_DEVICE, TRAFFIC_DETAIL_READ_ONLY,
  TRAFFIC_DETAIL_SCAN_TITLE, TRAFFIC_DETAIL_SHARE, TRAFFIC_DETAIL_WHERE, TRAFFIC_KIND_SHORT,
} from "../../constants/netaudit"
import type { TrafficPlace } from "../../types/netaudit"

// What one look kept about one address: its name, size, share and conversations. For a device on this network the helper's UDP scan and OS
// detection are offered here too (the same buttons as a device row, which show only when the helper can run them); an internet address, or one
// many devices share, is shown and never scanned. Names are text a peer chose, so they are shown as text only.
defineProps<{ place: TrafficPlace }>()
const emit = defineEmits<{ (e: "close"): void }>()
</script>

<template>
  <Panel :title="place.name || place.address" preserve-case data-test="traffic-detail-panel">
    <template #actions>
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="traffic-detail-close" @click="emit('close')">{{ TRAFFIC_DETAIL_CLOSE }}</BaseButton>
    </template>
    <div class="flex flex-col gap-3">
      <p class="m-0 break-all text-secondary">
        <span class="text-muted">{{ TRAFFIC_DETAIL_ADDRESS }}</span> <span class="font-mono" data-test="place-address">{{ place.address }}</span>
        · <span data-test="place-where">{{ place.internet ? TRAFFIC_DETAIL_WHERE.internet : TRAFFIC_DETAIL_WHERE.network }}</span>
      </p>
      <p class="m-0 text-secondary" data-test="place-share">{{ TRAFFIC_DETAIL_SHARE(place.percent, formatBytes(place.bytes)) }}</p>

      <div v-if="place.flows.length" class="overflow-x-auto">
        <table class="w-full min-w-[26rem] border-collapse text-left" data-test="place-flows">
          <thead>
            <tr class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">
              <th scope="col" class="py-1 pr-2 font-normal">{{ TRAFFIC_COLUMNS.port }}</th>
              <th scope="col" class="px-2 py-1 font-normal">{{ TRAFFIC_COLUMNS.kind }}</th>
              <th scope="col" class="px-2 py-1 text-right font-normal">{{ TRAFFIC_COLUMNS.sent }}</th>
              <th scope="col" class="py-1 pl-2 text-right font-normal">{{ TRAFFIC_COLUMNS.received }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in place.flows" :key="`${f.port}|${f.proto}`" class="border-t border-hairline" data-test="place-flow">
              <td class="py-1 pr-2 text-muted">{{ f.port || "·" }}/{{ f.proto }}</td>
              <td class="px-2 py-1 text-muted">{{ TRAFFIC_KIND_SHORT[f.kind] }}</td>
              <td class="px-2 py-1 text-right text-secondary">{{ formatBytes(f.bytes_out) }}</td>
              <td class="py-1 pl-2 text-right text-secondary">{{ formatBytes(f.bytes_in) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="m-0 text-[length:var(--text-small)] text-muted" data-test="place-no-flows">{{ TRAFFIC_DETAIL_NO_FLOWS }}</p>

      <div class="flex flex-col gap-2 border-t border-hairline pt-3">
        <template v-if="place.scannable">
          <p class="m-0 text-[length:var(--text-small)] text-muted">{{ TRAFFIC_DETAIL_SCAN_TITLE }}</p>
          <DeviceProbe :ip="place.address" />
        </template>
        <p v-else-if="place.internet" class="m-0 text-[length:var(--text-small)] text-muted" data-test="place-read-only">{{ TRAFFIC_DETAIL_READ_ONLY }}</p>
        <p v-else class="m-0 text-[length:var(--text-small)] text-muted" data-test="place-shared">{{ TRAFFIC_DETAIL_NOT_A_DEVICE }}</p>
      </div>
    </div>
  </Panel>
</template>
