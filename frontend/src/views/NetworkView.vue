<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, ref, watch } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import NetworkOverviewPanel from "../components/netaudit/NetworkOverviewPanel.vue"
import NetauditToolbar from "../components/netaudit/NetauditToolbar.vue"
import ReadinessBanner from "../components/netaudit/ReadinessBanner.vue"
import TimelineTab from "../components/netaudit/TimelineTab.vue"
import TrafficTab from "../components/netaudit/TrafficTab.vue"
import AuditsPanel from "../components/netaudit/AuditsPanel.vue"
import NetworkSettings from "../components/netaudit/NetworkSettings.vue"
import DevicesCard from "../components/netaudit/DevicesCard.vue"
import CaptureWindowCard from "../components/netaudit/CaptureWindowCard.vue"
import NearbySection from "../components/netaudit/NearbySection.vue"
import FindingsReport from "../components/netaudit/FindingsReport.vue"
import NetauditHelp from "../components/netaudit/NetauditHelp.vue"
import { useDispatcher } from "../composables/automation/useDispatcher"
import { useNetauditViewMode } from "../composables/netaudit/useNetauditViewMode"
import { DEVICE_PROBLEMS_INJECTION, useDeviceProblems } from "../composables/netaudit/useDeviceProblems"
import { useNetworkOverview } from "../composables/netaudit/useNetworkOverview"
import { AUDITOR_PROFILE, AUDITS_SET_UP, NETWORK_FINDINGS_NEEDS_AUDIT, NETWORK_OVERVIEW_LABEL, NETWORK_SUBTITLE, NETWORK_TAB_LABELS, NETWORK_TITLE } from "../constants/netaudit"
import { toNetwork } from "../router/routes"
import { NETWORK_TABS } from "../types/routes"
import type { NetworkAudit } from "../types/netaudit"
import type { StackTabItem } from "../types/stacks"
import type { NetworkTab } from "../types/routes"

// Network: the home network as a security feature of its own, the first stack under the Stacks group. Overview (one verdict, the numbers,
// what changed), Devices (who is on it), Findings (the last audit), Traffic (a look at this computer's own traffic), Timeline (one list of what happened), Audits (the scans that run on a schedule),
// Modules (the switchboard: every module is switched here and nowhere else) and Settings (the limits and details of each). The audit itself stays an ordinary automation, and this page reads its results. Simple or
// Advanced, and Help, are in the page header and mean the same on every tab.
const route = useRoute()
const { advanced } = useNetauditViewMode()
const { automations, fetchAutomations } = useDispatcher()

const tab = computed<NetworkTab | null>(() => {
  const t = route.params.tab
  return typeof t === "string" && (NETWORK_TABS as readonly string[]).includes(t) ? (t as NetworkTab) : null
})
const tabs: readonly StackTabItem[] = [
  { id: "overview", label: NETWORK_OVERVIEW_LABEL, to: toNetwork() },
  ...NETWORK_TABS.map((t) => ({ id: t, label: NETWORK_TAB_LABELS[t], to: toNetwork(t) })),
]
const audit = computed<NetworkAudit | null>(() => {
  const a = automations.value.find((x) => x.profile === AUDITOR_PROFILE)
  return a ? { workspaceId: a.workspace, automation: a.name } : null
})

// False until the automations have been read, so the page never offers "set up" while an audit exists.
const automationsLoaded = ref(false)
const { overview, error, load, start, stop } = useNetworkOverview(audit)
// The open problems by device, for the device list and the traffic detail; read again when the audit is known and when the tab changes.
const deviceProblems = useDeviceProblems(() => audit.value)
provide(DEVICE_PROBLEMS_INJECTION, deviceProblems)
watch([() => audit.value?.automation, tab], () => void deviceProblems.load())

// The audit is found from the automations list, so the overview is read again once it is known.
watch(() => audit.value?.automation, () => void load())
onMounted(async () => {
  start()
  await fetchAutomations(true)
  automationsLoaded.value = true
})
onUnmounted(stop)
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="NETWORK_TITLE" :subtitle="NETWORK_SUBTITLE" subtitle-hidden-on-phone>
      <template #actions><NetauditToolbar /></template>
    </PageHeader>
    <StackTabs label="Network" :tabs="tabs" :active="tab ?? 'overview'" />
    <ReadinessBanner :here="tab === 'settings' ? 'network-scan' : undefined" />

    <template v-if="tab === null">
      <ErrorState v-if="error && !overview" title="The overview could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!overview" label="Reading the network" :rows="3" />
      <NetworkOverviewPanel v-else :overview="overview" :has-audit="audit !== null" @open-tab="$router.push(toNetwork($event))" />
    </template>

    <template v-else-if="tab === 'devices'">
      <DevicesCard />
      <CaptureWindowCard v-if="advanced" />
      <NearbySection />
    </template>

    <template v-else-if="tab === 'findings'">
      <FindingsReport v-if="audit" :workspace-id="audit.workspaceId" :automation="audit.automation" />
      <LoadingState v-else-if="!automationsLoaded" label="Looking for your audit" />
      <EmptyState v-else title="No findings yet" :body="NETWORK_FINDINGS_NEEDS_AUDIT">
        <template #action><BaseButton variant="primary" @click="$router.push(toNetwork('audits'))">{{ AUDITS_SET_UP }}</BaseButton></template>
      </EmptyState>
    </template>

    <TrafficTab v-else-if="tab === 'traffic'" @open-tab="$router.push(toNetwork($event))" />
    <TimelineTab v-else-if="tab === 'timeline'" :audit="audit" />
    <AuditsPanel v-else-if="tab === 'audits'" />
    <StackModules v-else-if="tab === 'modules'" stack="network" />
    <NetworkSettings v-else />

    <NetauditHelp />
  </div>
</template>
