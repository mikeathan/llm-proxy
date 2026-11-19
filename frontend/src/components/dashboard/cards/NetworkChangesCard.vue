<script setup lang="ts">
import { computed } from "vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import { CARD_LIST_SCROLL } from "../../../constants/dashboard"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useNetworkSnapshot } from "../../../composables/netaudit/useNetworkSnapshot"
import { changeText } from "../../../utils/netaudit/networkOverview"
import { formatRelativeTime } from "../../../utils/format/time"
import { NETWORK_RECENT_NONE } from "../../../constants/netaudit"

// Dashboard card · What changed: the latest changes on the network (a new device, a port that opened), newest first. Small shows the
// latest one; medium and large show the list the server sends, scrolling inside the card.
const size = useCardSize()
const { overview, error } = useNetworkSnapshot()
const shown = computed(() => overview.value?.recent.slice(0, size.value === "xs" ? 1 : undefined) ?? [])
</script>

<template>
  <div data-test="network-changes-card">
    <ErrorState v-if="error && !overview" title="The network could not be read" :cause="error" next="It is read again in a minute." />
    <LoadingState v-else-if="!overview" label="Reading the network" :rows="2" />
    <p v-else-if="!overview.recent.length" class="m-0 text-muted">{{ NETWORK_RECENT_NONE }}</p>
    <ul v-else :class="['m-0 list-none p-0', CARD_LIST_SCROLL[size]]" aria-label="Recent changes">
      <li v-for="c in shown" :key="`${c.kind}|${c.at}|${c.mac ?? c.ip}`" class="flex flex-wrap items-baseline justify-between gap-2 border-b border-hairline/60 py-2 last:border-b-0">
        <span class="min-w-0 break-words text-primary">{{ changeText(c) }}</span>
        <span class="text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(c.at) }}</span>
      </li>
    </ul>
  </div>
</template>
