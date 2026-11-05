<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import StatCard from "../common/display/StatCard.vue"
import Sparkline from "../common/display/Sparkline.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import AttackPathsCard from "./AttackPathsCard.vue"
import { attentionReasons, changeText, hasRouterCounts, nextAction, postureHeadline, refusedToday, seriesOf } from "../../utils/netaudit/networkOverview"
import { formatRelativeTime, isRecorded } from "../../utils/format/time"
import { ACTIVITY_SEE_ALL, ROUTER_STAT_CAPTION, ROUTER_STAT_LABEL, NETWORK_CHECK_PROBLEM, NETWORK_NO_AUDIT, NETWORK_POSTURE_UNKNOWN_NEXT, NETWORK_RECENT_NONE, NETWORK_RECENT_TITLE, NETWORK_TREND_NOTE } from "../../constants/netaudit"
import type { NetworkOverview, NetworkPosture } from "../../types/netaudit"
import type { NetworkTab } from "../../types/routes"
import type { StatusState } from "../../types/ui"

// The Network page's first screen: one verdict, four figures with their trend, and what changed lately. It only shows
// what the server already knows, so it adds no scanning.
const props = defineProps<{ overview: NetworkOverview; hasAudit: boolean }>()
// The page owns navigation; this card only says where to go.
const emit = defineEmits<{ (e: "open-tab", tab: NetworkTab): void }>()

const NO_VALUE = "–"
const POSTURE_TAG: Record<NetworkPosture, { state: StatusState; label: string }> = {
  unknown: { state: "neutral", label: "Not looked at" },
  quiet: { state: "success", label: "Quiet" },
  attention: { state: "warning", label: "Needs a look" },
}
const reasons = computed(() => attentionReasons(props.overview))
const tag = computed(() => POSTURE_TAG[props.overview.posture])
const next = computed(() => nextAction(props.overview, props.hasAudit))
const devicesTrend = computed(() => seriesOf(props.overview.series, (d) => d.devices))
const findingsTrend = computed(() => seriesOf(props.overview.series.filter((d) => d.audited), (d) => d.findings))
const showRouter = computed(() => hasRouterCounts(props.overview.series))
const refused = computed(() => refusedToday(props.overview.series))
const refusedTrend = computed(() => seriesOf(props.overview.series, (d) => d.blocked ?? 0))
const lastAudit = computed(() => (props.overview.findings.scanned && isRecorded(props.overview.findings.scanned_at) ? formatRelativeTime(props.overview.findings.scanned_at) : "Never"))
const lastCheck = computed(() => (isRecorded(props.overview.presence.last_check) ? formatRelativeTime(props.overview.presence.last_check) : NO_VALUE))
// The problems tile says how many are serious and how many ports are open, the two numbers the Findings tab starts with.
const problemsCaption = computed(() => {
  const f = props.overview.findings
  return [`${f.serious} serious`, f.ports !== undefined ? `${f.ports} open ${f.ports === 1 ? "port" : "ports"}` : ""].filter(Boolean).join(" · ")
})
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
        <BaseButton v-if="next" variant="primary" class-name="self-start max-sm:!h-11" data-test="next-action" @click="emit('open-tab', next.tab)">{{ next.label }}</BaseButton>
      </div>
    </Panel>

    <AttackPathsCard v-if="overview.attack_paths_on && overview.findings.scanned" :paths="overview.attack_paths ?? []" />

    <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-px border border-hairline bg-border-hairline" data-test="stats">
      <StatCard label="Devices" :value="String(overview.devices.known)" :caption="devicesCaption">
        <Sparkline :values="devicesTrend" label="Devices known" unit="devices" state="info" />
      </StatCard>
      <StatCard label="Problems" :value="hasAudit ? String(overview.findings.open) : NO_VALUE" :caption="hasAudit ? problemsCaption : NETWORK_NO_AUDIT">
        <Sparkline v-if="hasAudit" :values="findingsTrend" label="Problems" unit="problems" state="success" />
      </StatCard>
      <StatCard v-if="showRouter" :label="ROUTER_STAT_LABEL" :value="refused.toLocaleString('en-US')" :caption="ROUTER_STAT_CAPTION" data-test="router-stat">
        <Sparkline :values="refusedTrend" label="Attempts the router refused" unit="a day" state="info" />
      </StatCard>
      <StatCard label="Last audit" :value="lastAudit" plain />
      <StatCard label="New-device check" :value="lastCheck" plain :caption="isRecorded(overview.presence.last_check) ? 'last look' : 'has not looked yet'" />
    </div>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ NETWORK_TREND_NOTE }}</p>

    <Panel :title="NETWORK_RECENT_TITLE" flush>
      <template #actions>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="see-all-activity" @click="emit('open-tab', 'timeline')">{{ ACTIVITY_SEE_ALL }}</BaseButton>
      </template>
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
