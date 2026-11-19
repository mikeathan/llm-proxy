<script setup lang="ts">
import { computed, provide, ref } from "vue"
import FindingsReport from "../../netaudit/FindingsReport.vue"
import AttackPathsCard from "../../netaudit/AttackPathsCard.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import SegmentedControl from "../../common/forms/SegmentedControl.vue"
import { useNetworkAudit } from "../../../composables/netaudit/useNetworkAudit"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"
import { IN_DASHBOARD_CARD } from "../../../constants/dashboard"
import { AUDITS_SET_UP, FINDINGS_VIEW_LABEL, FINDINGS_VIEW_OPTIONS, NETWORK_FINDINGS_NEEDS_AUDIT } from "../../../constants/netaudit"
import { toNetworkCard } from "../../../router/routes"
// Dashboard card · Findings: what the last audit found, and the decision about each (accept it, mark it fixed). The same findings can be read as
// the ways in they open (where each path starts, what it passes through and the one fix), one switch away; that view is only offered once an audit
// has run with the attack-paths module on. The audit is the one the owner set up; without one the card says so and points at the Audits card.
const { audit, loaded } = useNetworkAudit()
const { overview } = useNetworkSnapshot()
const view = ref("list")
const hasPaths = computed(() => !!overview.value?.attack_paths_on && !!overview.value.findings.scanned)
const showPaths = computed(() => view.value === "paths" && hasPaths.value)
// The ways in draw their own frame and title; the report is as it was.
provide(IN_DASHBOARD_CARD, false)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-findings-card">
    <SegmentedControl v-if="hasPaths" v-model="view" :options="FINDINGS_VIEW_OPTIONS" :label="FINDINGS_VIEW_LABEL" variant="compact" data-test="findings-view" />
    <AttackPathsCard v-if="showPaths" :paths="overview?.attack_paths ?? []" data-test="network-attack-paths-card" />
    <FindingsReport v-else-if="audit" :workspace-id="audit.workspaceId" :automation="audit.automation" />
    <LoadingState v-else-if="!loaded" label="Looking for your audit" />
    <EmptyState v-else title="No findings yet" :body="NETWORK_FINDINGS_NEEDS_AUDIT">
      <template #action><BaseButton variant="primary" @click="$router.push(toNetworkCard('network-audits'))">{{ AUDITS_SET_UP }}</BaseButton></template>
    </EmptyState>
  </div>
</template>
