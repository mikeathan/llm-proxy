<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import TrafficChart from "./TrafficChart.vue"
import TrafficDetail from "./TrafficDetail.vue"
import TrafficFlows from "./TrafficFlows.vue"
import TrafficPlaces from "./TrafficPlaces.vue"
import TrafficWhere from "./TrafficWhere.vue"
import { useDeviceNames } from "../../composables/netaudit/useDeviceNames"
import { sampleNote, splitPercent, stoppedNote, trafficPlace, trafficSentence } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_BUSIEST, TRAFFIC_DIRECTION_SPLIT, TRAFFIC_INTERNET_SPLIT, TRAFFIC_PRIVACY_NOTE, TRAFFIC_SERIES_TITLE, TRAFFIC_SUMMARY_STATS, TRAFFIC_WHAT_TITLE,
  TRAFFIC_WHERE_TITLE, TRAFFIC_WHERE_VIEW_LABELS, TRAFFIC_WHERE_VIEWS_LABEL,
} from "../../constants/netaudit"
import type { ChoiceOption } from "../../types/ui"
import type { HostTraffic, TrafficWhereView } from "../../types/netaudit"

// What one look at this computer's own traffic saw, in three parts: a summary (a sentence, the four totals and the seconds as a chart), where it
// went (the places, or the same internet traffic by operator or by country) and what it was (the kinds, and the conversations). Names come from DNS
// answers and TLS handshakes and are shown as text only. Nothing here is a finding. A place or a conversation opens its detail (and, for a device
// on this network, the helper scans).
const props = defineProps<{ traffic: HostTraffic }>()

const sentence = computed(() => trafficSentence(props.traffic))
const notes = computed(() => [stoppedNote(props.traffic), sampleNote(props.traffic)].filter(Boolean))
const direction = computed(() => splitPercent(props.traffic.bytes_in, props.traffic.bytes_out))
const reach = computed(() => splitPercent(props.traffic.internet_bytes, props.traffic.local_network_bytes))
const showSplit = computed(() => props.traffic.bytes_in + props.traffic.bytes_out > 0)
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

// Operators and countries are about the internet traffic, so they are offered only when some went there.
const views = computed<ChoiceOption[]>(() => {
  const all: TrafficWhereView[] = props.traffic.internet_bytes > 0 ? ["places", "operators", "countries"] : ["places"]
  return all.map((value) => ({ value, label: TRAFFIC_WHERE_VIEW_LABELS[value] }))
})
const view = ref<TrafficWhereView>("places")
const hasWhere = computed(() => props.traffic.destinations.length > 0)

// What is open belongs to one look: a new look starts clean.
const openAddress = ref<string | null>(null)
watch(() => props.traffic, () => { openAddress.value = null; view.value = "places" })
const place = computed(() => (openAddress.value ? trafficPlace(props.traffic, openAddress.value) : null))
const openPlace = (address: string) => { openAddress.value = address }

// What the person calls each device on this network, so a row can say "Pixel 8" under 192.168.50.10. A failed read leaves addresses alone.
const { names, load } = useDeviceNames()
onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-report">
    <Panel>
      <div class="flex flex-col gap-3">
        <div class="flex flex-col gap-1">
          <p class="m-0 text-[15px] font-semibold text-primary" data-test="traffic-headline">{{ sentence.headline }}</p>
          <p v-if="sentence.detail" class="m-0 text-secondary" data-test="traffic-detail">{{ sentence.detail }}</p>
        </div>
        <ul v-if="notes.length" class="m-0 flex list-none flex-col gap-1 p-0 text-[length:var(--text-small)] text-state-running" data-test="traffic-notes">
          <li v-for="n in notes" :key="n" class="break-words">{{ n }}</li>
        </ul>
        <dl v-if="showSplit" class="m-0 grid grid-cols-2 gap-3 sm:grid-cols-4" data-test="traffic-stats">
          <div v-for="s in stats" :key="s.id" class="flex flex-col gap-0.5">
            <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ s.label }}</dt>
            <dd class="m-0 font-mono text-[length:var(--text-numeral)] font-medium tabular-nums leading-[1.1] text-primary" :data-test="`stat-${s.id}`">{{ s.value }}</dd>
          </div>
        </dl>
        <p v-if="showSplit" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="traffic-split">{{ TRAFFIC_DIRECTION_SPLIT(direction[0], direction[1]) }} {{ TRAFFIC_INTERNET_SPLIT(reach[0], reach[1]) }}</p>
        <div v-if="traffic.per_second.length" class="flex flex-col gap-1 border-t border-hairline pt-3" :aria-label="TRAFFIC_SERIES_TITLE">
          <TrafficChart :per-second="traffic.per_second" />
          <p v-if="busiest" class="m-0 text-[length:var(--text-small)] text-muted">{{ busiest }}</p>
        </div>
      </div>
    </Panel>

    <TrafficDetail v-if="place" :place="place" @close="openAddress = null" />

    <Panel v-if="hasWhere || traffic.internet_bytes > 0" :title="TRAFFIC_WHERE_TITLE">
      <template v-if="views.length > 1" #actions>
        <SegmentedControl v-model="view" :options="views" :label="TRAFFIC_WHERE_VIEWS_LABEL" variant="compact" data-test="where-views" />
      </template>
      <TrafficPlaces v-if="view === 'places'" :traffic="traffic" :device-names="names" :open-address="openAddress" @open="openPlace" />
      <TrafficWhere v-else :traffic="traffic" :view="view" />
    </Panel>

    <Panel v-if="traffic.flows.length || traffic.kinds.length" :title="TRAFFIC_WHAT_TITLE">
      <TrafficFlows :traffic="traffic" @open="openPlace" />
    </Panel>

    <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-privacy">{{ TRAFFIC_PRIVACY_NOTE }}</p>
  </div>
</template>
