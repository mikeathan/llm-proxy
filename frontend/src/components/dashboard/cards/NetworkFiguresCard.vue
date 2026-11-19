<script setup lang="ts">
import { computed } from "vue"
import StatCard from "../../common/display/StatCard.vue"
import Sparkline from "../../common/display/Sparkline.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"
import { hasRouterCounts, refusedToday, seriesOf } from "../../../utils/netaudit/networkOverview"
import { formatRelativeTime, isRecorded } from "../../../utils/format/time"
import { NETWORK_NO_AUDIT, NETWORK_TREND_NOTE, PROBLEMS_SEE_ALL, ROUTER_STAT_CAPTION, ROUTER_STAT_LABEL } from "../../../constants/netaudit"
import { toNetworkCard } from "../../../router/routes"

// Dashboard card · Network figures: the devices and problems with their 30-day trend, the router's refused attempts, and when the audit and
// the new-device check last looked. It only shows what the server already knows, so it adds no scanning.
const NO_VALUE = "–"
const { overview, error, hasAudit } = useNetworkSnapshot()
const devicesTrend = computed(() => (overview.value ? seriesOf(overview.value.series, (d) => d.devices) : []))
const findingsTrend = computed(() => (overview.value ? seriesOf(overview.value.series.filter((d) => d.audited), (d) => d.findings) : []))
const showRouter = computed(() => !!overview.value && hasRouterCounts(overview.value.series))
const refused = computed(() => (overview.value ? refusedToday(overview.value.series) : 0))
const refusedTrend = computed(() => (overview.value ? seriesOf(overview.value.series, (d) => d.blocked ?? 0) : []))
const lastAudit = computed(() => (overview.value?.findings.scanned && isRecorded(overview.value.findings.scanned_at) ? formatRelativeTime(overview.value.findings.scanned_at) : "Never"))
const lastCheck = computed(() => (overview.value && isRecorded(overview.value.presence.last_check) ? formatRelativeTime(overview.value.presence.last_check) : NO_VALUE))
const problemsCaption = computed(() => {
  const f = overview.value?.findings
  if (!f) return ""
  return [`${f.serious} serious`, f.ports !== undefined ? `${f.ports} open ${f.ports === 1 ? "port" : "ports"}` : ""].filter(Boolean).join(" · ")
})
const devicesCaption = computed(() => {
  const d = overview.value?.devices
  if (!d) return ""
  return [`${d.active} seen in the last day`, d.unnamed ? `${d.unnamed} unnamed` : ""].filter(Boolean).join(" · ")
})
</script>

<template>
  <div data-test="network-figures-card">
    <ErrorState v-if="error && !overview" title="The network could not be read" :cause="error" next="It is read again in a minute." />
    <LoadingState v-else-if="!overview" label="Reading the network" :rows="2" />
    <template v-else>
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-px border border-hairline bg-border-hairline" data-test="stats">
        <StatCard label="Devices" :value="String(overview.devices.known)" :caption="devicesCaption"><Sparkline :values="devicesTrend" label="Devices known" unit="devices" state="info" /></StatCard>
        <StatCard label="Problems" :value="hasAudit ? String(overview.findings.open) : NO_VALUE" :caption="hasAudit ? problemsCaption : NETWORK_NO_AUDIT">
          <Sparkline v-if="hasAudit" :values="findingsTrend" label="Problems" unit="problems" state="success" />
          <RouterLink v-if="hasAudit && overview.findings.open > 0" :to="toNetworkCard('network-findings')" class="inline-flex min-h-8 items-center self-start text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline max-sm:min-h-11" data-test="problems-link">{{ PROBLEMS_SEE_ALL }}</RouterLink>
        </StatCard>
        <StatCard v-if="showRouter" :label="ROUTER_STAT_LABEL" :value="refused.toLocaleString('en-US')" :caption="ROUTER_STAT_CAPTION" data-test="router-stat"><Sparkline :values="refusedTrend" label="Attempts the router refused" unit="a day" state="info" /></StatCard>
        <StatCard label="Last audit" :value="lastAudit" plain />
        <StatCard label="New-device check" :value="lastCheck" plain :caption="isRecorded(overview.presence.last_check) ? 'last look' : 'has not looked yet'" />
      </div>
      <p class="m-0 mt-2 text-[length:var(--text-small)] text-muted">{{ NETWORK_TREND_NOTE }}</p>
    </template>
  </div>
</template>
