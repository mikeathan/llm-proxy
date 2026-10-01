<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import BaseButton from "../common/buttons/BaseButton.vue"
import RunActivityPill from "./RunActivityPill.vue"
import HostStats from "./HostStats.vue"
import BrandMark from "./BrandMark.vue"
import { PRODUCT_NAME } from "../../config/brand"
import { toOverview } from "../../router/routes"

// The top strip: where you are, plus the host stats and the global run pill,
// visible on every destination (plan D10). Navigation lives in AppSidebar; on mobile the strip
// also carries the brand and the drawer toggle, since the rail is hidden.

defineProps<{
  mobile: boolean
  drawerOpen: boolean
}>()

defineEmits<{ (e: "toggle-drawer"): void }>()

const route = useRoute()
const title = computed(() => route.meta.title ?? "")
</script>

<template>
  <header class="sticky top-0 z-30 flex h-12 flex-none items-center gap-3 border-b border-hairline bg-canvas px-3 lg:px-6">
    <template v-if="mobile">
      <BaseButton
        data-test="drawer-toggle"
        variant="ghost"
        icon="menu"
        icon-only
        label="Open navigation"
        aria-controls="app-sidebar"
        :aria-expanded="drawerOpen"
        @click="$emit('toggle-drawer')"
      />
      <RouterLink
        :to="toOverview()"
        :aria-label="`${PRODUCT_NAME} home`"
        class="flex flex-none items-center gap-2 rounded-[var(--radius-sm)] font-mono text-[13px] font-semibold text-primary focus-visible:outline-none focus-visible:ring-2"
      >
        <BrandMark />
        <span class="hidden sm:inline">{{ PRODUCT_NAME }}</span>
      </RouterLink>
      <span class="text-decorative" aria-hidden="true">/</span>
    </template>

    <nav aria-label="Breadcrumb" class="min-w-0 flex-1 truncate font-mono text-[length:var(--text-small)]">
      <span aria-current="page" class="text-primary">{{ title }}</span>
    </nav>

    <HostStats />
    <!-- Shell-owned controls, e.g. the run-notification bell (App.vue). -->
    <slot name="actions" />
    <RunActivityPill />
  </header>
</template>
