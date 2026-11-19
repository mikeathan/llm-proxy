<script setup lang="ts">
import { computed, provide, watch } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import NetauditToolbar from "../components/netaudit/NetauditToolbar.vue"
import ReadinessBanner from "../components/netaudit/ReadinessBanner.vue"
import TrafficTab from "../components/netaudit/TrafficTab.vue"
import AuditsPanel from "../components/netaudit/AuditsPanel.vue"
import NetworkSettings from "../components/netaudit/NetworkSettings.vue"
import DevicesCard from "../components/netaudit/DevicesCard.vue"
import CaptureWindowCard from "../components/netaudit/CaptureWindowCard.vue"
import NearbySection from "../components/netaudit/NearbySection.vue"
import FindingsReport from "../components/netaudit/FindingsReport.vue"
import NetauditHelp from "../components/netaudit/NetauditHelp.vue"
import { useNetworkAudit } from "../composables/netaudit/useNetworkAudit"
import { useNetauditViewMode } from "../composables/netaudit/useNetauditViewMode"
import { DEVICE_PROBLEMS_INJECTION, useDeviceProblems } from "../composables/netaudit/useDeviceProblems"
import { AUDITS_SET_UP, NETWORK_FINDINGS_NEEDS_AUDIT, NETWORK_SUBTITLE, NETWORK_TAB_LABELS, NETWORK_TITLE } from "../constants/netaudit"
import { toNetwork, toNetworkSettings } from "../router/routes"
import { NETWORK_SETTINGS_GROUPS, NETWORK_TABS } from "../types/routes"
import type { StackTabItem } from "../types/stacks"
import type { NetworkSettingsGroup, NetworkTab } from "../types/routes"

// Network: the home network as a security feature of its own, the first stack under the Stacks group. The page is where it is controlled:
// Devices (who is on it, and what to decide about each), Findings (the last audit, and what to decide about each), Traffic (the look at this
// computer's own traffic and the router setup), Audits (the scans that run on a schedule), Modules (the switchboard: every
// module is switched here and nowhere else) and Settings (the limits and details of each). The overview (the verdict, the figures, what
// changed) is on the dashboard's Network board, which the header links to. The audit itself stays an ordinary automation, and this page reads
// its results. Simple or Advanced, and Help, are in the page header and mean the same on every tab.
const route = useRoute()
const { advanced } = useNetauditViewMode()
const { audit, loaded: automationsLoaded } = useNetworkAudit()

const tab = computed<NetworkTab>(() => {
  const t = route.params.tab
  return typeof t === "string" && (NETWORK_TABS as readonly string[]).includes(t) ? (t as NetworkTab) : NETWORK_TABS[0]
})
// The group of the Settings tab; a missing or unknown `?group=` opens the first.
const settingsGroup = computed<NetworkSettingsGroup>(() => {
  const g = route.query.group
  return typeof g === "string" && (NETWORK_SETTINGS_GROUPS as readonly string[]).includes(g) ? (g as NetworkSettingsGroup) : NETWORK_SETTINGS_GROUPS[0]
})
// The page without a tab is Devices, the first of them.
const tabs: readonly StackTabItem[] = NETWORK_TABS.map((t) => ({ id: t, label: NETWORK_TAB_LABELS[t], to: toNetwork(t) }))
// The open problems by device, for the device list and the traffic detail; read again when the audit is known and when the tab changes.
const deviceProblems = useDeviceProblems(() => audit.value)
provide(DEVICE_PROBLEMS_INJECTION, deviceProblems)
watch([() => audit.value?.automation, tab], () => void deviceProblems.load())
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="NETWORK_TITLE" :subtitle="NETWORK_SUBTITLE" subtitle-hidden-on-phone>
      <template #actions><NetauditToolbar /></template>
    </PageHeader>
    <StackTabs label="Network" :tabs="tabs" :active="tab" />
    <StackBoardLink stack="network" />
    <ReadinessBanner :here="tab === 'settings' ? 'network-scan' : undefined" />

    <template v-if="tab === 'devices'">
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

    <TrafficTab v-else-if="tab === 'traffic'" @open-tab="$router.push($event === 'settings' ? toNetworkSettings('helper') : toNetwork($event))" />
    <AuditsPanel v-else-if="tab === 'audits'" />
    <StackModules v-else-if="tab === 'modules'" stack="network" />
    <NetworkSettings v-else :group="settingsGroup" />

    <NetauditHelp />
  </div>
</template>
