<script setup lang="ts">
import { computed, onMounted, ref, toRef } from "vue"
import type { NavGroup, NavId } from "../../types/routes"
import { usePersistedState } from "../../composables/ui/usePersistedState"
import { useShellNavigation } from "../../composables/ui/useShellNavigation"
import { AdminApiService } from "../../services/admin/adminService"
import { PRODUCT_NAME } from "../../config/brand"
import Icon from "../icons/Icon.vue"
import BrandMark from "./BrandMark.vue"
import NotificationDot from "../common/NotificationDot.vue"
import SidebarLink from "./SidebarLink.vue"

// The left rail on a wide screen (plan D10, amended by frontend-redesign-v2): the
// destinations from the shared navigation, Assistant first, expanded ↔ collapsed
// (persisted). On a narrow screen the bottom tab bar takes its place. The current
// item comes from the route (see utils/navigation.ts), not from state kept here.

const props = defineProps<{
  /** Unread run notifications per entry (plan Phase 4). */
  badges?: Partial<Record<NavId, number>>
}>()

const { entries, home } = useShellNavigation(toRef(() => props.badges ?? {}))

const COLLAPSED_KEY = "sidebar-collapsed"
const collapsed = usePersistedState<boolean>(COLLAPSED_KEY, {
  version: 1,
  fallback: false,
  parse: (data) => (typeof data === "boolean" ? data : null),
})

// Groups the operator folded (persisted). A group holding the open page never reads as closed,
// so the current destination is always visible; a collapsed rail lists the destinations flat.
const CLOSED_GROUPS_KEY = "sidebar-closed-groups"
const closedGroups = usePersistedState<string[]>(CLOSED_GROUPS_KEY, {
  version: 1,
  fallback: [],
  parse: (data) => (Array.isArray(data) && data.every((id) => typeof id === "string") ? data : null),
})
const isGroupOpen = (group: NavGroup) => group.current || !closedGroups.value.includes(group.id)
const toggleGroup = (group: NavGroup) => {
  closedGroups.value = isGroupOpen(group) ? [...closedGroups.value, group.id] : closedGroups.value.filter((id) => id !== group.id)
}
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
      <template v-for="entry in entries" :key="entry.kind === 'item' ? entry.item.id : entry.group.id">
        <SidebarLink v-if="entry.kind === 'item'" :item="entry.item" :collapsed="isCollapsed" />
        <template v-else>
          <button
            v-if="!isCollapsed"
            type="button"
            :data-test="`group-${entry.group.id}`"
            :aria-controls="`group-${entry.group.id}`"
            :aria-expanded="isGroupOpen(entry.group)"
            :class="[
              'flex h-8 items-center gap-2.5 rounded-[var(--radius-md)] px-2.5 font-mono text-[length:var(--text-small)] transition-colors duration-fast ease-standard hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2',
              entry.group.current ? 'text-primary' : 'text-muted',
            ]"
            @click="toggleGroup(entry.group)"
          >
            <span class="relative flex-none">
              <Icon :name="entry.group.icon" size="sm" class-name="flex-none" />
              <NotificationDot v-if="entry.group.badge && !isGroupOpen(entry.group)" :count="entry.group.badge" />
            </span>
            <span class="flex-1 text-left">{{ entry.group.label }}</span>
            <Icon name="chevron-down" size="xs" :class-name="['flex-none transition-transform duration-fast', isGroupOpen(entry.group) ? '' : '-rotate-90'].join(' ')" />
          </button>
          <div v-if="isCollapsed || isGroupOpen(entry.group)" :id="`group-${entry.group.id}`" class="flex flex-col gap-0.5">
            <SidebarLink v-for="child in entry.group.items" :key="child.id" :item="child" :collapsed="isCollapsed" nested />
          </div>
        </template>
      </template>
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
