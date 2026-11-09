<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import TrafficChart from "./TrafficChart.vue"
import { destinationLabel, kindShares, sampleNote, splitPercent, stoppedNote, trafficSentence } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_BUSIEST, TRAFFIC_COLUMNS, TRAFFIC_DESTINATIONS_TITLE, TRAFFIC_DIRECTION_SPLIT, TRAFFIC_FLOWS_TITLE, TRAFFIC_INTERNET_SPLIT, TRAFFIC_KIND_LABELS,
  TRAFFIC_KIND_SHORT, TRAFFIC_KINDS_TITLE, TRAFFIC_NO_NAME, TRAFFIC_OTHERS, TRAFFIC_PRIVACY_NOTE, TRAFFIC_SERIES_TITLE,
} from "../../constants/netaudit"
import type { HostTraffic } from "../../types/netaudit"

// What one look at this computer's own traffic saw, as a sentence, the places the data went, the kinds of traffic, a chart of the seconds and
// the conversations. Names come from DNS answers and are shown as text only. Nothing here is a finding.
const props = defineProps<{ traffic: HostTraffic }>()

const sentence = computed(() => trafficSentence(props.traffic))
const shares = computed(() => kindShares(props.traffic.kinds))
const notes = computed(() => [stoppedNote(props.traffic), sampleNote(props.traffic)].filter(Boolean))
// Each bar is a share of the biggest place, so the list reads as a ranking.
const biggest = computed(() => Math.max(1, ...props.traffic.destinations.map((d) => d.bytes)))
const barWidth = (bytes: number) => `${Math.max(2, Math.round((bytes / biggest.value) * 100))}%`
const otherDestinations = computed(() => props.traffic.other_flows)
const direction = computed(() => splitPercent(props.traffic.bytes_in, props.traffic.bytes_out))
const reach = computed(() => splitPercent(props.traffic.internet_bytes, props.traffic.local_network_bytes))
const busiest = computed(() => {
  const series = props.traffic.per_second
  const max = Math.max(0, ...series)
  return max > 0 ? TRAFFIC_BUSIEST(series.indexOf(max), formatBytes(max)) : ""
})
const showSplit = computed(() => props.traffic.bytes_in + props.traffic.bytes_out > 0)
const flowName = (f: HostTraffic["flows"][number]) => f.name || f.address
</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-report">
    <div class="flex flex-col gap-1 rounded-[var(--radius-md)] bg-surface-raised p-4">
      <p class="m-0 text-[15px] font-semibold text-primary" data-test="traffic-headline">{{ sentence.headline }}</p>
      <p v-if="sentence.detail" class="m-0 text-secondary" data-test="traffic-detail">{{ sentence.detail }}</p>
    </div>

    <ul v-if="notes.length" class="m-0 flex list-none flex-col gap-1 p-0 text-[length:var(--text-small)] text-state-running" data-test="traffic-notes">
      <li v-for="n in notes" :key="n" class="break-words">{{ n }}</li>
    </ul>

    <div v-if="traffic.destinations.length || shares.length" class="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
      <Panel v-if="traffic.destinations.length" :title="TRAFFIC_DESTINATIONS_TITLE">
        <ul class="m-0 flex list-none flex-col gap-3 p-0" data-test="destinations">
          <li v-for="d in traffic.destinations" :key="d.address + (d.name ?? '')" class="flex flex-col gap-1" data-test="destination">
            <span class="flex items-baseline justify-between gap-3">
              <span class="min-w-0 break-all text-primary"><span data-test="destination-name">{{ destinationLabel(d).text }}</span><span v-if="!destinationLabel(d).named" class="ml-1 text-muted">({{ TRAFFIC_NO_NAME }})</span></span>
              <span class="flex-none text-[length:var(--text-small)] text-muted">{{ formatBytes(d.bytes) }}</span>
            </span>
            <span class="block h-2 bg-surface-hover" aria-hidden="true"><span class="block h-2 bg-accent-info" :style="{ width: barWidth(d.bytes) }"></span></span>
          </li>
        </ul>
        <p v-if="otherDestinations" class="m-0 mt-3 text-[length:var(--text-small)] text-muted" data-test="destinations-other">{{ TRAFFIC_OTHERS(otherDestinations) }} · {{ formatBytes(traffic.other_bytes) }}</p>
      </Panel>

      <Panel v-if="shares.length" :title="TRAFFIC_KINDS_TITLE">
        <ul class="m-0 flex list-none flex-col gap-2 p-0" data-test="kinds">
          <li v-for="k in shares" :key="k.kind" class="flex items-baseline justify-between gap-3" data-test="kind">
            <span class="min-w-0 break-words text-primary">{{ TRAFFIC_KIND_LABELS[k.kind] }}</span>
            <span class="flex-none text-[length:var(--text-small)] text-muted">{{ k.percent }}% · {{ formatBytes(k.bytes) }}</span>
          </li>
        </ul>
        <p v-if="showSplit" class="m-0 mt-3 border-t border-hairline pt-2 text-[length:var(--text-small)] text-secondary" data-test="traffic-split">
          {{ TRAFFIC_DIRECTION_SPLIT(direction[0], direction[1]) }} {{ TRAFFIC_INTERNET_SPLIT(reach[0], reach[1]) }}
        </p>
      </Panel>
    </div>

    <Panel v-if="traffic.per_second.length" :title="TRAFFIC_SERIES_TITLE">
      <div class="flex flex-col gap-2">
        <TrafficChart :per-second="traffic.per_second" />
        <p v-if="busiest" class="m-0 text-[length:var(--text-small)] text-muted">{{ busiest }}</p>
      </div>
    </Panel>

    <Panel v-if="traffic.flows.length" :title="TRAFFIC_FLOWS_TITLE" flush>
      <div class="overflow-x-auto">
        <table class="w-full min-w-[34rem] border-collapse text-left" data-test="flows">
          <thead>
            <tr class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">
              <th scope="col" class="px-4 py-2 font-normal">{{ TRAFFIC_COLUMNS.destination }}</th>
              <th scope="col" class="px-2 py-2 font-normal">{{ TRAFFIC_COLUMNS.port }}</th>
              <th scope="col" class="px-2 py-2 font-normal">{{ TRAFFIC_COLUMNS.kind }}</th>
              <th scope="col" class="px-2 py-2 text-right font-normal">{{ TRAFFIC_COLUMNS.sent }}</th>
              <th scope="col" class="px-4 py-2 text-right font-normal">{{ TRAFFIC_COLUMNS.received }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in traffic.flows" :key="`${f.address}|${f.port}|${f.proto}`" class="border-t border-hairline" data-test="flow">
              <td class="max-w-[22rem] break-all px-4 py-2 text-primary">{{ flowName(f) }}</td>
              <td class="px-2 py-2 text-muted">{{ f.port || "·" }}</td>
              <td class="px-2 py-2 text-muted">{{ TRAFFIC_KIND_SHORT[f.kind] }}</td>
              <td class="px-2 py-2 text-right text-secondary">{{ formatBytes(f.bytes_out) }}</td>
              <td class="px-4 py-2 text-right text-secondary">{{ formatBytes(f.bytes_in) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </Panel>

    <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-privacy">{{ TRAFFIC_PRIVACY_NOTE }}</p>
  </div>
</template>
