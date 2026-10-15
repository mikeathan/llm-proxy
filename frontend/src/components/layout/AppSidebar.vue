<script setup lang="ts">
import { computed, onMounted, ref, toRef } from "vue"
import type { NavId, NavItem } from "../../types/routes"
import { usePersistedState } from "../../composables/ui/usePersistedState"
import { useShellNavigation } from "../../composables/ui/useShellNavigation"
import { AdminApiService } from "../../services/admin/adminService"
import { PRODUCT_NAME } from "../../config/brand"
import Icon from "../icons/Icon.vue"
import BrandMark from "./BrandMark.vue"
import NotificationDot from "../common/NotificationDot.vue"

// The left rail on a wide screen (plan D10, amended by frontend-redesign-v2): the
// destinations from the shared navigation, Assistant first, expanded ↔ collapsed
// (persisted). On a narrow screen the bottom tab bar takes its place. The current
// item comes from the route (see utils/navigation.ts), not from state kept here.

const props = defineProps<{
  /** Unread run notifications per entry (plan Phase 4). */
  badges?: Partial<Record<NavId, number>>
}>()

const { items, home } = useShellNavigation(toRef(() => props.badges ?? {}))

const COLLAPSED_KEY = "sidebar-collapsed"
const collapsed = usePersistedState<boolean>(COLLAPSED_KEY, {
  version: 1,
  fallback: false,
  parse: (data) => (typeof data === "boolean" ? data : null),
})

const accessibleName = (item: NavItem) =>
  item.badge ? `${item.label}, ${item.badge} unread run notification${item.badge === 1 ? "" : "s"}` : item.label
const isCollapsed = computed(() => collapsed.value)

const version = ref<string | null>(null)
onMounted(async () => {
  try {
    version.value = (await AdminApiService.fetchVersion()).version ?? null
  } catch {
    // Non-critical: the badge is omitted.
  }
})
</script>

<template>
  <aside
    id="app-sidebar"
    aria-label="Primary"
    :data-collapsed="isCollapsed"
    :class="['sticky top-0 flex h-screen flex-none flex-col bg-surface-raised transition-[width] duration-slow ease-standard', isCollapsed ? 'w-14' : 'w-[216px]']"
  >
    <div class="flex h-12 flex-none items-center gap-2.5 overflow-hidden px-4">
      <RouterLink
        :to="home"
        :aria-label="`${PRODUCT_NAME} home`"
        class="flex min-w-0 items-center gap-2.5 rounded-[var(--radius-sm)] font-mono text-[13px] font-semibold tracking-[0.02em] text-primary focus-visible:outline-none focus-visible:ring-2"
      >
        <BrandMark />
        <span v-if="!isCollapsed" class="truncate">{{ PRODUCT_NAME }}</span>
      </RouterLink>
      <span v-if="version && !isCollapsed" class="flex-none font-mono text-[length:var(--text-micro)] text-muted">{{ version }}</span>
    </div>

    <nav class="flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-3">
      <RouterLink
        v-for="item in items"
        :key="item.id"
        :to="item.to"
        :data-destination="item.id"
        :aria-label="accessibleName(item)"
        :title="isCollapsed ? item.label : undefined"
        :aria-current="item.current ? 'page' : undefined"
        :class="[
          'flex h-8 items-center gap-2.5 rounded-[var(--radius-md)] px-2.5 font-mono text-[length:var(--text-small)] transition-colors duration-fast ease-standard focus-visible:outline-none focus-visible:ring-2',
          item.current ? 'bg-surface-active text-primary shadow-[inset_2px_0_0_rgb(var(--accent-brand))]' : 'text-muted hover:bg-surface-hover hover:text-primary',
        ]"
      >
        <span class="relative flex-none">
          <Icon :name="item.icon" size="sm" class-name="flex-none" />
          <NotificationDot v-if="item.badge" :count="item.badge" />
        </span>
        <span v-if="!isCollapsed" aria-hidden="true">{{ item.label }}</span>
      </RouterLink>
    </nav>

    <div class="flex-none p-2">
      <button
        type="button"
        data-test="sidebar-collapse"
        aria-controls="app-sidebar"
        :aria-expanded="!isCollapsed"
        :aria-label="isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        :title="isCollapsed ? 'Expand sidebar' : undefined"
        class="flex h-8 w-full items-center gap-2.5 rounded-[var(--radius-md)] px-2.5 font-mono text-[length:var(--text-small)] text-muted transition-colors duration-fast hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2"
        @click="collapsed = !collapsed"
      >
        <Icon name="collapse" size="sm" :class-name="['flex-none transition-transform duration-slow', isCollapsed ? 'rotate-180' : ''].join(' ')" />
        <span v-if="!isCollapsed">Collapse</span>
      </button>
    </div>
  </aside>
</template>
