<script setup lang="ts">
import LoadingState from "../../common/feedback/LoadingState.vue"
import AttackPathsCard from "../../netaudit/AttackPathsCard.vue"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"

// Dashboard card · Ways in: for each path where it starts, the findings it passes through and the one fix. It needs an audit that has run.
const { overview } = useNetworkSnapshot()
</script>

<template>
  <LoadingState v-if="!overview" label="Reading the network" :rows="2" />
  <AttackPathsCard v-else-if="overview.attack_paths_on && overview.findings.scanned" :paths="overview.attack_paths ?? []" data-test="network-attack-paths-card" />
  <p v-else class="m-0 text-muted" data-test="network-attack-paths-off">Ways in appear here after an audit has run with the attack-paths module on.</p>
</template>
