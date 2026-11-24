<script setup lang="ts">
import ScrollStrip from "../common/layout/ScrollStrip.vue"
import type { StackTabItem } from "../../types/stacks"

// A stack page's tabs. Each is its own route, so a deep link and the back button work. `active` is the id of the tab that is
// open; the first item is the page without a tab (the overview, the activity).
defineProps<{ label: string; tabs: readonly StackTabItem[]; active: string }>()
</script>

<template>
  <ScrollStrip :label="label" :current="active">
    <RouterLink v-for="tab in tabs" :key="tab.id" :to="tab.to" :aria-current="active === tab.id ? 'page' : undefined" class="stack-tab">{{ tab.label }}<template v-if="tab.badge && tab.badge.count > 0"><span class="stack-tab-dot" aria-hidden="true" data-test="tab-dot" /><span class="sr-only">, {{ tab.badge.words }}</span></template></RouterLink>
  </ScrollStrip>
</template>

<style scoped lang="postcss">
.stack-tab {
  @apply inline-flex h-11 flex-none items-center border-b-2 border-transparent px-3.5 text-muted no-underline hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}
.stack-tab-dot {
  @apply ml-2 inline-block h-2 w-2 flex-none rounded-full bg-accent-brand;
}
.stack-tab[aria-current="page"] {
  @apply border-accent-brand font-medium text-primary;
}
</style>
