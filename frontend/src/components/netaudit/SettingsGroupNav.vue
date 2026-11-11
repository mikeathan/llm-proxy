<script setup lang="ts">
import ScrollStrip from "../common/layout/ScrollStrip.vue"
import { toNetworkSettings } from "../../router/routes"
import { SETTINGS_GROUP_COPY, SETTINGS_GROUPS_LABEL } from "../../constants/netaudit"
import { NETWORK_SETTINGS_GROUPS } from "../../types/routes"
import type { NetworkSettingsGroup } from "../../types/routes"

// The groups of the Settings tab as a row of pills under the page's own tabs. Each is a link, so a group has an address, the back button works and
// a warning elsewhere can open the group that holds its fix.
defineProps<{ active: NetworkSettingsGroup }>()
</script>

<template>
  <ScrollStrip :label="SETTINGS_GROUPS_LABEL" :current="active">
    <RouterLink v-for="g in NETWORK_SETTINGS_GROUPS" :key="g" :to="toNetworkSettings(g)" :aria-current="active === g ? 'page' : undefined" class="settings-pill" :data-test="`settings-group-${g}`">{{ SETTINGS_GROUP_COPY[g].label }}</RouterLink>
  </ScrollStrip>
</template>

<style scoped lang="postcss">
.settings-pill {
  @apply inline-flex h-11 flex-none items-center rounded-full border border-hairline px-3.5 text-[length:var(--text-small)] text-muted no-underline hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8;
}
.settings-pill[aria-current="page"] {
  @apply border-accent-brand bg-surface-hover font-medium text-primary;
}
</style>
