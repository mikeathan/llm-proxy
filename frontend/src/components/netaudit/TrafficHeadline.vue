<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import TrafficChart from "./TrafficChart.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { sampleNote, selfLine, stoppedNote, trafficSentence } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import { TRAFFIC_BUSIEST, TRAFFIC_ONLY_THIS, TRAFFIC_SERIES_TITLE, TRAFFIC_SUMMARY_STATS } from "../../constants/netaudit"
import type { HostTraffic } from "../../types/netaudit"

// The sentence a look at this computer says first: how much was sent and received, this program's own page set apart, that it is this computer only,
// and (Advanced) the four totals and the per-second chart. It sits at the top of this computer's page, above its map and places.
const props = defineProps<{ traffic: HostTraffic }>()
const { advanced } = useNetauditViewMode()

const sentence = computed(() => trafficSentence(props.traffic))
const self = computed(() => selfLine(props.traffic))
const notes = computed(() => [stoppedNote(props.traffic), sampleNote(props.traffic)].filter(Boolean))
const stats = computed(() => [
  { id: "sent", label: TRAFFIC_SUMMARY_STATS.sent, value: formatBytes(props.traffic.bytes_out) },
  { id: "received", label: TRAFFIC_SUMMARY_STATS.received, value: formatBytes(props.traffic.bytes_in) },
  { id: "internet", label: TRAFFIC_SUMMARY_STATS.internet, value: formatBytes(props.traffic.internet_bytes) },
  { id: "network", label: TRAFFIC_SUMMARY_STATS.network, value: formatBytes(props.traffic.local_network_bytes) },
])
const busiest = computed(() => {
  const series = props.traffic.per_second
  const max = Math.max(0, ...series)
  return max > 0 ? TRAFFIC_BUSIEST(series.indexOf(max), formatBytes(max)) : ""
})
</script>

<template>
  <div data-test="traffic-report-head">
    <Panel>
      <div class="flex flex-col gap-3">
        <div class="flex flex-col gap-1">
          <p class="m-0 text-[15px] font-semibold text-primary" data-test="traffic-headline">{{ sentence.headline }}</p>
          <p v-if="sentence.detail" class="m-0 text-secondary" data-test="traffic-detail">{{ sentence.detail }}</p>
        </div>
        <p v-if="self" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="traffic-self">{{ self }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-only-this">{{ TRAFFIC_ONLY_THIS }}</p>
        <ul v-if="notes.length" class="m-0 flex list-none flex-col gap-1 p-0 text-[length:var(--text-small)] text-state-running" data-test="traffic-notes">
          <li v-for="n in notes" :key="n" class="break-words">{{ n }}</li>
        </ul>
        <template v-if="advanced">
          <dl class="m-0 grid grid-cols-2 gap-3 border-t border-hairline pt-3 sm:grid-cols-4" data-test="traffic-stats">
            <div v-for="s in stats" :key="s.id" class="flex flex-col gap-0.5">
              <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ s.label }}</dt>
              <dd class="m-0 font-mono text-[length:var(--text-numeral)] font-medium tabular-nums leading-[1.1] text-primary" :data-test="`stat-${s.id}`">{{ s.value }}</dd>
            </div>
          </dl>
          <div v-if="traffic.per_second.length" class="flex flex-col gap-1" :aria-label="TRAFFIC_SERIES_TITLE" data-test="traffic-series">
            <TrafficChart :per-second="traffic.per_second" />
            <p v-if="busiest" class="m-0 text-[length:var(--text-small)] text-muted">{{ busiest }}</p>
          </div>
        </template>
      </div>
    </Panel>
  </div>
</template>
