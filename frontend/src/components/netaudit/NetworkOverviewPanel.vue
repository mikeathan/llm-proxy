<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import StatCard from "../common/display/StatCard.vue"
import Sparkline from "../common/display/Sparkline.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { attentionReasons, changeText, postureHeadline, seriesOf } from "../../utils/netaudit/networkOverview"
import { formatRelativeTime } from "../../utils/format/time"
import { NETWORK_CHECK_PROBLEM, NETWORK_NO_AUDIT, NETWORK_POSTURE_UNKNOWN_NEXT, NETWORK_RECENT_NONE, NETWORK_RECENT_TITLE, NETWORK_TREND_NOTE } from "../../constants/netaudit"
import type { NetworkOverview, NetworkPosture } from "../../types/netaudit"
import type { StatusState } from "../../types/ui"

// The Network page's first screen: one verdict, four figures with their trend, and what changed lately. It only shows
// what the server already knows, so it adds no scanning.
const props = defineProps<{ overview: NetworkOverview; hasAudit: boolean }>()

const NO_VALUE = "–"
const POSTURE_TAG: Record<NetworkPosture, { state: StatusState; label: string }> = {
  unknown: { state: "neutral", label: "Not looked at" },
  quiet: { state: "success", label: "Quiet" },
  attention: { state: "warning", label: "Needs a look" },
}
const reasons = computed(() => attentionReasons(props.overview))
const tag = computed(() => POSTURE_TAG[props.overview.posture])
const devicesTrend = computed(() => seriesOf(props.overview.series, (d) => d.devices))
const findingsTrend = computed(() => seriesOf(props.overview.series.filter((d) => d.audited), (d) => d.findings))
const lastAudit = computed(() => (props.overview.findings.scanned && props.overview.findings.scanned_at ? formatRelativeTime(props.overview.findings.scanned_at) : "Never"))
const lastCheck = computed(() => (props.overview.presence.last_check ? formatRelativeTime(props.overview.presence.last_check) : NO_VALUE))
const devicesCaption = computed(() => {
  const d = props.overview.devices
  return [`${d.active} seen in the last day`, d.unnamed ? `${d.unnamed} unnamed` : ""].filter(Boolean).join(" · ")
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel>
      <div class="flex flex-col gap-2" data-test="posture">
        <div class="flex flex-wrap items-center gap-2">
          <StatusTag :state="tag.state" :label="tag.label" />
          <h2 class="m-0 text-[length:var(--text-heading)] font-semibold text-primary">{{ postureHeadline(overview) }}</h2>
        </div>
        <ul v-if="reasons.length" class="m-0 flex list-none flex-col gap-1 p-0" aria-label="What needs a look">
          <li v-for="r in reasons" :key="r" class="text-secondary">{{ r }}</li>
        </ul>
        <p v-else-if="overview.posture === 'unknown'" class="m-0 text-muted">{{ NETWORK_POSTURE_UNKNOWN_NEXT }}</p>
        <p v-if="overview.presence.problem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ NETWORK_CHECK_PROBLEM }}: {{ overview.presence.problem }}</p>
      </div>
    </Panel>

    <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-px border border-hairline bg-border-hairline" data-test="stats">
      <StatCard label="Devices" :value="String(overview.devices.known)" :caption="devicesCaption">
        <Sparkline :values="devicesTrend" label="Devices known" unit="devices" state="info" />
      </StatCard>
      <StatCard label="Open findings" :value="hasAudit ? String(overview.findings.open) : NO_VALUE" :caption="hasAudit ? `${overview.findings.serious} serious · ${overview.findings.new} new` : NETWORK_NO_AUDIT">
        <Sparkline v-if="hasAudit" :values="findingsTrend" label="Open findings" unit="findings" state="success" />
      </StatCard>
      <StatCard label="Last audit" :value="lastAudit" />
      <StatCard label="New-device check" :value="lastCheck" :caption="overview.presence.last_check ? 'last look' : 'has not looked yet'" />
    </div>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ NETWORK_TREND_NOTE }}</p>

    <Panel :title="NETWORK_RECENT_TITLE" flush>
      <p v-if="!overview.recent.length" class="m-0 px-4 py-3 text-muted">{{ NETWORK_RECENT_NONE }}</p>
      <ul v-else class="m-0 flex list-none flex-col p-0" aria-label="Recent changes">
        <li v-for="c in overview.recent" :key="`${c.kind}|${c.at}|${c.mac ?? c.ip}`" class="flex flex-wrap items-baseline justify-between gap-2 border-b border-hairline/60 px-4 py-2 last:border-b-0">
          <span class="min-w-0 break-words text-primary">{{ changeText(c) }}</span>
          <span class="text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(c.at) }}</span>
        </li>
      </ul>
    </Panel>
  </div>
</template>
