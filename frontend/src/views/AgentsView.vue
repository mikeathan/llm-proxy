<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import AgentLockdown from "../components/agents/AgentLockdown.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import { AGENTS_LOCKDOWN_LABEL, AGENTS_SUBTITLE, AGENTS_TAB_LABELS, AGENTS_TITLE } from "../constants/stacks"
import { toAgents } from "../router/routes"
import { AGENTS_TABS } from "../types/routes"
import type { AgentsTab } from "../types/routes"
import type { StackTabItem } from "../types/stacks"

// Agents: the second stack. Lock down (the stack's one action) is the page; Modules is the switchboard for the parts that keep the record.
// The record of what the agents did is the Agent activity card on the dashboard's Agents board, which the header links to.
const route = useRoute()
const tab = computed<AgentsTab | null>(() => {
  const t = route.params.tab
  return typeof t === "string" && (AGENTS_TABS as readonly string[]).includes(t) ? (t as AgentsTab) : null
})
const tabs: readonly StackTabItem[] = [
  { id: "lockdown", label: AGENTS_LOCKDOWN_LABEL, to: toAgents() },
  ...AGENTS_TABS.map((t) => ({ id: t, label: AGENTS_TAB_LABELS[t], to: toAgents(t) })),
]
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="AGENTS_TITLE" :subtitle="AGENTS_SUBTITLE" subtitle-hidden-on-phone />
    <StackTabs :label="AGENTS_TITLE" :tabs="tabs" :active="tab ?? 'lockdown'" />
    <StackBoardLink stack="agents" />
    <StackModules v-if="tab === 'modules'" stack="agents" />
    <AgentLockdown v-else />
  </div>
</template>
