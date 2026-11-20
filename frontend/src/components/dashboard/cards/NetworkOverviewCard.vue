<script setup lang="ts">
import { computed } from "vue"
import { RouterLink, useRouter } from "vue-router"
import MicroLabel from "../../common/display/MicroLabel.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import OverviewVerdict from "../../netaudit/overview/OverviewVerdict.vue"
import OverviewFigures from "../../netaudit/overview/OverviewFigures.vue"
import OverviewChanges from "../../netaudit/overview/OverviewChanges.vue"
import StandingCard from "../../netaudit/StandingCard.vue"
import CoverageStrip from "../../netaudit/CoverageStrip.vue"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"
import { OVERVIEW_ALL_ACTIVITY, OVERVIEW_CHANGES_TITLE, OVERVIEW_COVERAGE_TITLE, OVERVIEW_STANDING_PENDING, STANDING_TITLE } from "../../../constants/netaudit"
import { toNetwork, toNetworkCard } from "../../../router/routes"

// Dashboard card · Network overview: how the network is, in one place. The verdict and the next step, what changed lately, the figures with their
// trend, whether it is getting better, and what is watching. They are one card because they are one read of the server (the overview), so the
// first screen of the Network tab answers "is it all right?" without a trip to five cards. The sections are plain headings, not boxes inside a box:
// a Panel in a card draws only its content. Wide (a large card on a screen from the tablet width up), the verdict and the changes sit side by side,
// and so do the trend and the coverage; on a phone they are one after another.
const size = useCardSize()
const router = useRouter()
const { overview } = useNetworkSnapshot()
const wide = computed(() => size.value === "lg")
const showStanding = computed(() => !!overview.value?.standing && overview.value.findings.scanned)
</script>

<template>
  <div class="flex flex-col gap-6" data-test="network-overview-card">
    <div :class="['grid gap-6', wide ? 'md:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]' : '']">
      <OverviewVerdict class="min-w-0" />
      <section class="flex min-w-0 flex-col gap-2" aria-labelledby="overview-changes-title">
        <div class="flex items-baseline justify-between gap-2">
          <MicroLabel id="overview-changes-title">{{ OVERVIEW_CHANGES_TITLE }}</MicroLabel>
          <RouterLink :to="toNetworkCard('network-timeline')" class="inline-flex min-h-8 items-center font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2 max-sm:min-h-11" data-test="all-activity">{{ OVERVIEW_ALL_ACTIVITY }} →</RouterLink>
        </div>
        <OverviewChanges />
      </section>
    </div>
    <OverviewFigures />
    <LoadingState v-if="!overview" label="Reading the network" :rows="1" />
    <div v-else :class="['grid gap-6', wide ? 'md:grid-cols-2' : '']">
      <section class="flex min-w-0 flex-col gap-2" aria-labelledby="overview-standing-title">
        <MicroLabel id="overview-standing-title">{{ STANDING_TITLE }}</MicroLabel>
        <StandingCard v-if="showStanding" :standing="overview.standing!" data-test="network-standing-card" />
        <p v-else class="m-0 text-muted" data-test="network-standing-none">{{ OVERVIEW_STANDING_PENDING }}</p>
      </section>
      <section class="flex min-w-0 flex-col gap-2" aria-labelledby="overview-coverage-title">
        <MicroLabel id="overview-coverage-title">{{ OVERVIEW_COVERAGE_TITLE }}</MicroLabel>
        <CoverageStrip data-test="network-coverage-card" @open-tab="router.push(toNetwork($event))" />
      </section>
    </div>
  </div>
</template>
