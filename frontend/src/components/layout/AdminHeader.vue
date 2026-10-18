<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import RunActivityPill from "./RunActivityPill.vue"
import HostStats from "./HostStats.vue"
import BrandMark from "./BrandMark.vue"
import WorkspaceSwitcher from "./WorkspaceSwitcher.vue"
import { PRODUCT_NAME } from "../../config/brand"
import { useLastWorkspace } from "../../composables/ui/useLastWorkspace"
import { homeLocation } from "../../utils/navigation"

// The top strip: where you are, plus the host stats and the global run pill,
// visible on every destination (plan D10). Navigation lives in AppSidebar (a wide
// screen) and BottomTabs (a phone); on a phone the strip also carries the brand,
// since the rail is hidden. The strip sits above the phone's bottom bar so the bottom
// sheets its controls open (stats, runs, workspaces) cover the bar.

defineProps<{
  mobile: boolean
}>()

const { lastWorkspace } = useLastWorkspace()
const home = computed(() => homeLocation(lastWorkspace.value))

const route = useRoute()
const title = computed(() => route.meta.title ?? "")
</script>

<template>
  <header class="sticky top-0 z-40 flex h-12 flex-none items-center gap-2 bg-canvas px-3 sm:gap-3 lg:px-6">
    <template v-if="mobile">
      <RouterLink
        :to="home"
        :aria-label="`${PRODUCT_NAME} home`"
        class="flex flex-none items-center gap-2 rounded-[var(--radius-sm)] font-mono text-[13px] font-semibold text-primary focus-visible:outline-none focus-visible:ring-2"
      >
        <BrandMark />
        <span class="hidden sm:inline">{{ PRODUCT_NAME }}</span>
      </RouterLink>
      <span class="text-decorative" aria-hidden="true">/</span>
    </template>

    <WorkspaceSwitcher :mobile="mobile" />
    <!-- On a phone the bottom bar already says where you are and the title was squeezed to "A…": it stays for
         screen readers, and the spacer keeps the stats on the right. -->
    <nav aria-label="Breadcrumb" :class="mobile ? 'sr-only' : 'min-w-0 flex-1 truncate font-mono text-[length:var(--text-small)]'">
      <span aria-current="page" class="text-primary">{{ title }}</span>
    </nav>
    <span v-if="mobile" class="flex-1" aria-hidden="true" />

    <HostStats />
    <!-- Shell-owned controls, e.g. the run-notification bell (App.vue). -->
    <slot name="actions" />
    <RunActivityPill />
  </header>
</template>
