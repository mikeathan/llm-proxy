<script setup lang="ts">
import { computed } from "vue"
import StatusTag from "../../common/display/StatusTag.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import { CARD_LIST_SCROLL } from "../../../constants/dashboard"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"
import { attentionReasons, nextAction, postureHeadline } from "../../../utils/netaudit/networkOverview"
import { NETWORK_CHECK_PROBLEM, NETWORK_POSTURE_UNKNOWN_NEXT } from "../../../constants/netaudit"
import type { NetworkPosture } from "../../../types/netaudit"
import type { StatusState } from "../../../types/ui"

// Network overview · the verdict: one line on how the network is, what needs a look and the next step, which links to where it is done.
// Small is the verdict and the count; medium adds the reasons in a list that scrolls; large adds the next step. It only reads what the server already knows.
const POSTURE_TAG: Record<NetworkPosture, { state: StatusState; label: string }> = {
  unknown: { state: "neutral", label: "Not looked at" },
  quiet: { state: "success", label: "Quiet" },
  attention: { state: "warning", label: "Needs a look" },
}
const size = useCardSize()
const { overview, error, hasAudit } = useNetworkSnapshot()
const tag = computed(() => (overview.value ? POSTURE_TAG[overview.value.posture] : null))
const reasons = computed(() => (overview.value ? attentionReasons(overview.value) : []))
const next = computed(() => (overview.value ? nextAction(overview.value, hasAudit.value) : null))
</script>

<template>
  <div data-test="network-overview-verdict">
    <ErrorState v-if="error && !overview" title="The network could not be read" :cause="error" next="It is read again in a minute." />
    <LoadingState v-else-if="!overview || !tag" label="Reading the network" :rows="2" />
    <template v-else>
      <div class="mb-2 flex flex-wrap items-center gap-2">
        <StatusTag :state="tag.state" :label="tag.label" />
        <span v-if="size === 'xs'" class="text-muted" data-test="posture-count">{{ reasons.length }} to look at</span>
      </div>
      <p v-if="size !== 'xs'" class="m-0 text-[length:var(--text-heading)] font-semibold text-primary">{{ postureHeadline(overview) }}</p>
      <ul v-if="size === 'sm' && reasons.length" :class="['m-0 mt-2 list-none p-0 text-secondary', CARD_LIST_SCROLL[size]]" aria-label="What needs a look">
        <li v-for="r in reasons" :key="r">{{ r }}</li>
      </ul>
      <div v-if="size === 'lg'">
        <ul v-if="reasons.length" class="m-0 mt-2 flex list-none flex-col gap-1 p-0 text-secondary" aria-label="What needs a look">
          <li v-for="r in reasons" :key="r">{{ r }}</li>
        </ul>
        <p v-else-if="overview.posture === 'unknown'" class="m-0 mt-2 text-muted">{{ NETWORK_POSTURE_UNKNOWN_NEXT }}</p>
        <p v-if="overview.presence.problem" role="alert" class="m-0 mt-2 text-[length:var(--text-small)] text-state-error">{{ NETWORK_CHECK_PROBLEM }}: {{ overview.presence.problem }}</p>
        <RouterLink v-if="next" :to="next.to" class="mt-3 inline-flex min-h-8 items-center font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2 max-sm:min-h-11" data-test="next-action">{{ next.label }} →</RouterLink>
      </div>
    </template>
  </div>
</template>
