<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import TrafficChart from "./TrafficChart.vue"
import TrafficDetail from "./TrafficDetail.vue"
import TrafficFlows from "./TrafficFlows.vue"
import TrafficMap from "./TrafficMap.vue"
import TrafficTalk from "./TrafficTalk.vue"
import TrafficWhere from "./TrafficWhere.vue"
import { useDeviceNames } from "../../composables/netaudit/useDeviceNames"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { mapDots, routePoints, sampleNote, selfLine, stoppedNote, talkGroups, trafficPlace, trafficSentence } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import { motionScroll } from "../../utils/motion"
import {
  TRAFFIC_ADVANCED_TITLE, TRAFFIC_BUSIEST, TRAFFIC_MAP_NO_CITIES, TRAFFIC_MAP_NO_POINTS, TRAFFIC_MAP_NOTE, TRAFFIC_MAP_ROUTE_NOTE, TRAFFIC_MAP_TITLE,
  TRAFFIC_ONLY_THIS, TRAFFIC_PRIVACY_NOTE, TRAFFIC_SERIES_TITLE, TRAFFIC_SUMMARY_STATS, TRAFFIC_TALK_TITLE, TRAFFIC_WHAT_TITLE, TRAFFIC_WHERE_VIEW_LABELS,
  TRAFFIC_WHERE_VIEWS_LABEL,
} from "../../constants/netaudit"
import type { ChoiceOption } from "../../types/ui"
import type { HostTraffic, TrafficWhereView } from "../../types/netaudit"

// What one look at this computer's own traffic saw, as answers to plain questions: how much went where (a sentence, with this program's own page
// set apart), where on the map the internet traffic went, and who the computer talked to (internet, this network, and broadcast chatter closed by
// default). A place opens its detail: who runs it, where it is used, and the route to it. The per-second chart, the totals, the operator and
// country lists and the conversation table are under Advanced. Names come from DNS answers and TLS handshakes and are shown as text only. Nothing
// here is a finding.
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

// What the person calls each device on this network, so a row can say "Pixel 8" for 192.168.50.10. A failed read leaves addresses alone.
const { names, load } = useDeviceNames()
onMounted(load)
const groups = computed(() => talkGroups(props.traffic, names.value))
const rows = computed(() => groups.value.flatMap((g) => g.rows))
const talkTotal = computed(() => rows.value.reduce((n, r) => n + r.bytes, 0))

// What is open belongs to one look: a new look starts clean.
const openAddress = ref<string | null>(null)
watch(() => props.traffic, () => { openAddress.value = null })
const place = computed(() => (openAddress.value ? trafficPlace(props.traffic, openAddress.value) : null))
const detailEl = ref<HTMLElement | null>(null)
async function openPlace(address: string) {
  openAddress.value = address
  await nextTick()
  detailEl.value?.scrollIntoView?.({ behavior: motionScroll("smooth"), block: "start" })
}

// The map: the internet places with a city, and the route of the open place when it has been traced.
const { traceOf } = useRouteTrace()
const dots = computed(() => mapDots(rows.value, openAddress.value))
const route = computed(() => routePoints(openAddress.value ? traceOf(openAddress.value) : null))
const hasInternet = computed(() => rows.value.some((r) => r.group === "internet"))
const hasCities = computed(() => rows.value.some((r) => r.owner?.city || r.owner?.region))

// Advanced: the same internet traffic by operator or by country.
const view = ref<TrafficWhereView>("operators")
const views: ChoiceOption[] = (["operators", "countries"] as const).map((value) => ({ value, label: TRAFFIC_WHERE_VIEW_LABELS[value] }))
</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-report">
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

    <div v-if="place" ref="detailEl" class="scroll-mt-4">
      <TrafficDetail :place="place" @close="openAddress = null" />
    </div>

    <Panel v-if="hasInternet" :title="TRAFFIC_MAP_TITLE">
      <div class="flex flex-col gap-2">
        <TrafficMap v-if="dots.length" :dots="dots" :route="route" @pick="openPlace" />
        <p v-else class="m-0 text-secondary" data-test="map-empty">{{ hasCities ? TRAFFIC_MAP_NO_POINTS : TRAFFIC_MAP_NO_CITIES }}</p>
        <p v-if="dots.length" class="m-0 text-[length:var(--text-small)] text-muted">{{ TRAFFIC_MAP_NOTE }}</p>
        <p v-if="route.length > 1" class="m-0 text-[length:var(--text-small)] text-muted" data-test="map-route-note">{{ TRAFFIC_MAP_ROUTE_NOTE }}</p>
      </div>
    </Panel>

    <Panel :title="TRAFFIC_TALK_TITLE">
      <TrafficTalk :groups="groups" :total="talkTotal" :open-address="openAddress" :other-flows="traffic.other_flows" :other-bytes="traffic.other_bytes" @open="openPlace" />
    </Panel>

    <template v-if="advanced">
      <Panel v-if="traffic.internet_bytes > 0" :title="TRAFFIC_ADVANCED_TITLE">
        <template #actions>
          <SegmentedControl v-model="view" :options="views" :label="TRAFFIC_WHERE_VIEWS_LABEL" variant="compact" data-test="where-views" />
        </template>
        <TrafficWhere :traffic="traffic" :view="view" />
      </Panel>
      <Panel v-if="traffic.flows.length || traffic.kinds.length" :title="TRAFFIC_WHAT_TITLE">
        <TrafficFlows :traffic="traffic" @open="openPlace" />
      </Panel>
    </template>

    <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-privacy">{{ TRAFFIC_PRIVACY_NOTE }}</p>
  </div>
</template>
