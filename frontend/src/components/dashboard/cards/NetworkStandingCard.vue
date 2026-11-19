<script setup lang="ts">
import LoadingState from "../../common/feedback/LoadingState.vue"
import StandingCard from "../../netaudit/StandingCard.vue"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"

// Dashboard card · Are we getting better: how long the open problems have lasted and how the last week compared with the one before.
const { overview } = useNetworkSnapshot()
</script>

<template>
  <LoadingState v-if="!overview" label="Reading the network" :rows="2" />
  <StandingCard v-else-if="overview.standing && overview.findings.scanned" :standing="overview.standing" data-test="network-standing-card" />
  <p v-else class="m-0 text-muted" data-test="network-standing-none">This fills in after the first audit has run.</p>
</template>
