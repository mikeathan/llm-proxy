<script setup lang="ts">
import FindingsReport from "../../netaudit/FindingsReport.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import { useNetworkAudit } from "../../../composables/netaudit/useNetworkAudit"
import { AUDITS_SET_UP, NETWORK_FINDINGS_NEEDS_AUDIT } from "../../../constants/netaudit"
import { toNetworkCard } from "../../../router/routes"

// Dashboard card · Findings: what the last audit found, and the decision about each (accept it, mark it fixed). The audit is the one the owner
// set up; without one the card says so and points at the Audits card.
const { audit, loaded } = useNetworkAudit()
</script>

<template>
  <FindingsReport v-if="audit" :workspace-id="audit.workspaceId" :automation="audit.automation" />
  <LoadingState v-else-if="!loaded" label="Looking for your audit" />
  <EmptyState v-else title="No findings yet" :body="NETWORK_FINDINGS_NEEDS_AUDIT">
    <template #action><BaseButton variant="primary" @click="$router.push(toNetworkCard('network-audits'))">{{ AUDITS_SET_UP }}</BaseButton></template>
  </EmptyState>
</template>
