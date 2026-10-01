<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue"
import { useRoute } from "vue-router"
import type { RouteLocationNamedRaw } from "vue-router"
import type { Destination } from "../../types/routes"
import { usePersistedState } from "../../composables/ui/usePersistedState"
import { AdminApiService } from "../../services/admin/adminService"
import { toActivity, toAutomations, toModels, toOverview, toSettings, toWorkspaces } from "../../router/routes"
import { PRODUCT_NAME } from "../../config/brand"
import Icon from "../icons/Icon.vue"
import BrandMark from "./BrandMark.vue"
import NotificationDot from "../common/NotificationDot.vue"

// Persistent left rail (plan D10): six destinations, expanded ↔ collapsed on
// desktop (persisted), an overlay drawer on mobile. The current item is derived
// from the matched route's `meta.destination` — there is no active-tab state.

const props = defineProps<{
  /** Below `lg`: render as an overlay drawer instead of a rail. */
  mobile: boolean
  /** Drawer visibility (mobile only). */
  open: boolean
  /** Unread run notifications per destination (plan Phase 4). */
  badges?: Partial<Record<Destination, number>>
}>()

const emit = defineEmits<{ (e: "update:open", open: boolean): void }>()

interface NavItem {
  id: Destination
  label: string
  icon: string
  to: RouteLocationNamedRaw
}

const NAV: NavItem[] = [
  { id: "overview", label: "Overview", icon: "nav-overview", to: toOverview() },
  { id: "workspaces", label: "Workspaces", icon: "nav-workspaces", to: toWorkspaces() },
  { id: "automations", label: "Automations", icon: "nav-automations", to: toAutomations() },
  { id: "models", label: "Models", icon: "nav-models", to: toModels() },
  { id: "activity", label: "Activity", icon: "nav-activity", to: toActivity() },
  { id: "settings", label: "Settings", icon: "nav-settings", to: toSettings() },
]

const COLLAPSED_KEY = "sidebar-collapsed"
const collapsedPref = usePersistedState<boolean>(COLLAPSED_KEY, {
  version: 1,
  fallback: false,
  parse: (data) => (typeof data === "boolean" ? data : null),
})
// The drawer always shows labels; collapse is a desktop-only preference.
const collapsed = computed(() => !props.mobile && collapsedPref.value)

const badgeFor = (id: Destination) => props.badges?.[id] ?? 0
const accessibleName = (item: NavItem) => {
  const unread = badgeFor(item.id)
  return unread ? `${item.label}, ${unread} unread run notification${unread === 1 ? "" : "s"}` : item.label
}

const route = useRoute()
const current = computed(() => route.meta.destination)

const version = ref<string | null>(null)
onMounted(async () => {
  try {
    version.value = (await AdminApiService.fetchVersion()).version ?? null
  } catch {
    // Non-critical: the badge is omitted.
  }
})

const panel = ref<HTMLElement | null>(null)
const close = () => emit("update:open", false)

// Navigating from the drawer closes it; opening it moves focus inside.
watch(() => route.fullPath, () => {
  if (props.mobile && props.open) close()
})
watch(() => props.open, async (open) => {
  if (!props.mobile || !open) return
  await nextTick()
  panel.value?.querySelector<HTMLElement>("nav a")?.focus()
})

function onKeydown(event: KeyboardEvent) {
  if (props.mobile && props.open && event.key === "Escape") close()
}
</script>

<template>
  <div
    v-if="mobile && open"
    data-test="drawer-scrim"
    class="fixed inset-0 z-[35] bg-scrim/60"
    aria-hidden="true"
    @click="close"
  ></div>
  <aside
    id="app-sidebar"
    ref="panel"
    aria-label="Primary"
    :data-collapsed="collapsed"
    :inert="mobile && !open ? true : undefined"
    :class="[
      'flex flex-col border-r border-hairline bg-canvas',
      mobile
        ? ['fixed inset-y-0 left-0 z-40 w-[264px] transition-transform duration-slow ease-emphasised', open ? 'translate-x-0' : '-translate-x-full']
        : ['sticky top-0 h-screen flex-none transition-[width] duration-slow ease-standard', collapsed ? 'w-14' : 'w-[216px]'],
    ]"
    @keydown="onKeydown"
  >
    <div class="flex h-12 flex-none items-center gap-2.5 overflow-hidden border-b border-hairline px-4">
      <RouterLink
        :to="toOverview()"
        :aria-label="`${PRODUCT_NAME} home`"
        class="flex min-w-0 items-center gap-2.5 rounded-[var(--radius-sm)] font-mono text-[13px] font-semibold tracking-[0.02em] text-primary focus-visible:outline-none focus-visible:ring-2"
      >
        <BrandMark />
        <span v-if="!collapsed" class="truncate">{{ PRODUCT_NAME }}</span>
      </RouterLink>
      <span
        v-if="version && !collapsed"
        class="flex-none font-mono text-[length:var(--text-micro)] text-muted"
      >{{ version }}</span>
    </div>

    <nav class="flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-3">
      <RouterLink
        v-for="item in NAV"
        :key="item.id"
        :to="item.to"
        :aria-label="accessibleName(item)"
        :title="collapsed ? item.label : undefined"
        :aria-current="current === item.id ? 'page' : undefined"
        :class="[
          'flex h-8 items-center gap-2.5 rounded-[var(--radius-md)] px-2.5 font-mono text-[length:var(--text-small)] transition-colors duration-fast ease-standard focus-visible:outline-none focus-visible:ring-2',
          current === item.id
            ? 'bg-surface-active text-primary shadow-[inset_2px_0_0_rgb(var(--accent-brand))]'
            : 'text-muted hover:bg-surface-hover hover:text-primary',
        ]"
      >
        <span class="relative flex-none">
          <Icon :name="item.icon" size="sm" class-name="flex-none" />
          <NotificationDot v-if="badgeFor(item.id)" :count="badgeFor(item.id)" />
        </span>
        <span v-if="!collapsed" aria-hidden="true">{{ item.label }}</span>
      </RouterLink>
    </nav>

    <div v-if="!mobile" class="flex-none border-t border-hairline p-2">
      <button
        type="button"
        data-test="sidebar-collapse"
        aria-controls="app-sidebar"
        :aria-expanded="!collapsed"
        :aria-label="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        :title="collapsed ? 'Expand sidebar' : undefined"
        class="flex h-8 w-full items-center gap-2.5 rounded-[var(--radius-md)] px-2.5 font-mono text-[length:var(--text-small)] text-muted transition-colors duration-fast hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2"
        @click="collapsedPref = !collapsedPref"
      >
        <Icon name="collapse" size="sm" :class-name="['flex-none transition-transform duration-slow', collapsed ? 'rotate-180' : ''].join(' ')" />
        <span v-if="!collapsed">Collapse</span>
      </button>
    </div>
  </aside>
</template>
