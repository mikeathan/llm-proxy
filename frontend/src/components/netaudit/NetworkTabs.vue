<script setup lang="ts">
import ScrollStrip from "../common/layout/ScrollStrip.vue"
import { toNetwork } from "../../router/routes"
import { NETWORK_OVERVIEW_LABEL, NETWORK_TAB_LABELS } from "../../constants/netaudit"
import { NETWORK_TABS } from "../../types/routes"
import type { NetworkTab } from "../../types/routes"

// The Network page's tabs. Each is its own route, so a deep link and the back button work; the overview is the page
// without a tab.
defineProps<{ active: NetworkTab | null }>()
</script>

<template>
  <ScrollStrip label="Network" :current="active ?? 'overview'">
    <RouterLink :to="toNetwork()" :aria-current="active === null ? 'page' : undefined" class="network-tab">{{ NETWORK_OVERVIEW_LABEL }}</RouterLink>
    <RouterLink v-for="tab in NETWORK_TABS" :key="tab" :to="toNetwork(tab)" :aria-current="active === tab ? 'page' : undefined" class="network-tab">{{ NETWORK_TAB_LABELS[tab] }}</RouterLink>
  </ScrollStrip>
</template>

<style scoped lang="postcss">
.network-tab {
  @apply inline-flex h-11 flex-none items-center border-b-2 border-transparent px-3.5 text-muted no-underline hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}
.network-tab[aria-current="page"] {
  @apply border-accent-brand font-medium text-primary;
}
</style>
