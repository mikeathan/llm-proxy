<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import NetauditToolbar from "../components/netaudit/NetauditToolbar.vue"
import ReadinessBanner from "../components/netaudit/ReadinessBanner.vue"
import NetworkSettings from "../components/netaudit/NetworkSettings.vue"
import NetauditHelp from "../components/netaudit/NetauditHelp.vue"
import { NETWORK_SUBTITLE, NETWORK_TAB_LABELS, NETWORK_TITLE } from "../constants/netaudit"
import { toNetwork } from "../router/routes"
import { NETWORK_SETTINGS_GROUPS, NETWORK_TABS } from "../types/routes"
import type { StackTabItem } from "../types/stacks"
import type { NetworkSettingsGroup, NetworkTab } from "../types/routes"

// Network: the home network as a security feature of its own, the first stack under the Stacks group. This page is only where it is controlled:
// Modules (the switchboard: every module is switched here and nowhere else) and Settings (the limits and details of each). Everything to be
// seen or decided about a device, a finding, a scan or a look is a card on the dashboard's Network board, which the header links to; the old
// tab addresses (/network/devices and the others) open it. The audit itself stays an ordinary automation. Simple or Advanced, and Help, are in
// the page header and mean the same wherever a card shows.
const route = useRoute()

const tab = computed<NetworkTab>(() => {
  const t = route.params.tab
  return typeof t === "string" && (NETWORK_TABS as readonly string[]).includes(t) ? (t as NetworkTab) : NETWORK_TABS[0]
})
// The group of the Settings tab; a missing or unknown `?group=` opens the first.
const settingsGroup = computed<NetworkSettingsGroup>(() => {
  const g = route.query.group
  return typeof g === "string" && (NETWORK_SETTINGS_GROUPS as readonly string[]).includes(g) ? (g as NetworkSettingsGroup) : NETWORK_SETTINGS_GROUPS[0]
})
// The page without a tab is Modules, the first of them.
const tabs: readonly StackTabItem[] = NETWORK_TABS.map((t) => ({ id: t, label: NETWORK_TAB_LABELS[t], to: toNetwork(t) }))
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="NETWORK_TITLE" :subtitle="NETWORK_SUBTITLE" subtitle-hidden-on-phone>
      <template #actions><NetauditToolbar /></template>
    </PageHeader>
    <StackTabs label="Network" :tabs="tabs" :active="tab" />
    <StackBoardLink stack="network" />
    <ReadinessBanner :here="tab === 'settings' ? 'network-scan' : undefined" />

    <StackModules v-if="tab === 'modules'" stack="network" />
    <NetworkSettings v-else :group="settingsGroup" />

    <NetauditHelp />
  </div>
</template>
