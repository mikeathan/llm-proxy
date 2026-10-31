<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import NetworkTabs from "../components/netaudit/NetworkTabs.vue"
import NetworkOverviewPanel from "../components/netaudit/NetworkOverviewPanel.vue"
import NetauditToolbar from "../components/netaudit/NetauditToolbar.vue"
import ReadinessBanner from "../components/netaudit/ReadinessBanner.vue"
import AuditList from "../components/netaudit/AuditList.vue"
import DevicesCard from "../components/netaudit/DevicesCard.vue"
import DecoysCard from "../components/netaudit/DecoysCard.vue"
import FindingsReport from "../components/netaudit/FindingsReport.vue"
import NetauditHelp from "../components/netaudit/NetauditHelp.vue"
import BrowserAlertsPanel from "../components/netaudit/BrowserAlertsPanel.vue"
import ToggleField from "../components/common/forms/ToggleField.vue"
import { useDispatcher } from "../composables/automation/useDispatcher"
import { useNetworkOverview } from "../composables/netaudit/useNetworkOverview"
import { useNetauditSwitch } from "../composables/netaudit/useNetauditSwitch"
import { NetauditService } from "../services/netaudit/netauditService"
import { AUDITOR_PROFILE, NETWORK_AUDITS_TITLE, NETWORK_FINDINGS_NEEDS_AUDIT, NETWORK_NO_AUDIT_PANEL, NETWORK_SET_UP_AUDIT, NETWORK_SETUP_LINK, NETWORK_SUBTITLE, NETWORK_TITLE, SUMMARY_HINT, SUMMARY_LABEL, SUMMARY_LOAD_FAILED, SUMMARY_SAVE_FAILED } from "../constants/netaudit"
import { toSettings } from "../router/routes"
import { NETWORK_TABS } from "../types/routes"
import type { NetworkAudit } from "../types/netaudit"
import type { NetworkTab } from "../types/routes"

// Network: the home network as a security feature of its own. Overview (one verdict, the numbers, what changed), Devices
// (who is on it, the new-device check, decoys) and Findings (the last audit). What may be scanned stays in Settings →
// Network scan; the audit itself stays an ordinary automation, and this page reads its results.
const route = useRoute()
const { automations, fetchAutomations } = useDispatcher()

const tab = computed<NetworkTab | null>(() => {
  const t = route.params.tab
  return typeof t === "string" && (NETWORK_TABS as readonly string[]).includes(t) ? (t as NetworkTab) : null
})
const audit = computed<NetworkAudit | null>(() => {
  const a = automations.value.find((x) => x.profile === AUDITOR_PROFILE)
  return a ? { workspaceId: a.workspace, automation: a.name } : null
})

// False until the automations have been read, so the page never offers "set up" while an audit exists.
const automationsLoaded = ref(false)
const summary = useNetauditSwitch(() => NetauditService.weeklySummary(), (on) => NetauditService.setWeeklySummary(on), { load: SUMMARY_LOAD_FAILED, save: SUMMARY_SAVE_FAILED })
const { overview, error, load, start, stop } = useNetworkOverview(audit)

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
    <PageHeader :title="NETWORK_TITLE" :subtitle="NETWORK_SUBTITLE" />
    <NetworkTabs :active="tab" />
    <ReadinessBanner />

    <template v-if="tab === null">
      <ErrorState v-if="error && !overview" title="The overview could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!overview" label="Reading the network" :rows="3" />
      <template v-else>
        <NetworkOverviewPanel :overview="overview" :has-audit="audit !== null" />
        <Panel title="Weekly summary">
          <div class="flex flex-col gap-2">
            <ToggleField :model-value="summary.enabled.value" :label="SUMMARY_LABEL" :hint="SUMMARY_HINT" :disabled="summary.busy.value" @update:model-value="summary.set" />
            <p v-if="summary.error.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ summary.error.value }}</p>
          </div>
        </Panel>
        <BrowserAlertsPanel />
        <Panel :title="NETWORK_AUDITS_TITLE">
          <div class="flex flex-col gap-3">
            <AuditList />
            <div v-if="automationsLoaded && !audit" class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between" data-test="no-audit">
              <p class="m-0 min-w-0 flex-1 text-secondary">{{ NETWORK_NO_AUDIT_PANEL }}</p>
              <BaseButton variant="primary" class-name="max-sm:!h-11" @click="$router.push(toSettings('network-scan'))">{{ NETWORK_SET_UP_AUDIT }}</BaseButton>
            </div>
            <RouterLink :to="toSettings('network-scan')" class="self-start text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2">{{ NETWORK_SETUP_LINK }}</RouterLink>
          </div>
        </Panel>
      </template>
    </template>

    <template v-else-if="tab === 'devices'">
      <NetauditToolbar />
      <DevicesCard />
      <DecoysCard />
    </template>

    <template v-else-if="audit">
      <NetauditToolbar />
      <FindingsReport :workspace-id="audit.workspaceId" :automation="audit.automation" />
    </template>
    <EmptyState v-else title="No findings yet" :body="NETWORK_FINDINGS_NEEDS_AUDIT">
      <template #action><BaseButton variant="primary" @click="$router.push(toSettings('network-scan'))">Set up a network audit</BaseButton></template>
    </EmptyState>

    <NetauditHelp />
  </div>
</template>
