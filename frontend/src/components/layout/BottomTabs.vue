<script setup lang="ts">
import { computed, nextTick, ref, toRef, watch } from "vue"
import { useRoute } from "vue-router"
import type { NavId, NavItem } from "../../types/routes"
import { useShellNavigation } from "../../composables/ui/useShellNavigation"
import Icon from "../icons/Icon.vue"
import NotificationDot from "../common/NotificationDot.vue"

// The phone's navigation (plan frontend-redesign-v2, V6): four tabs within thumb's
// reach — Assistant, Workspace, Automations, Reminders — and More, a sheet for the
// rest. Built from the same navigation as the sidebar, so they cannot drift apart.
// The sheet closes on Escape, on its scrim and after a destination is chosen, and
// focus returns to More.
const props = defineProps<{
  badges?: Partial<Record<NavId, number>>
}>()

const { items } = useShellNavigation(toRef(() => props.badges ?? {}))
const tabItems = computed(() => items.value.filter((i) => i.tab))
const moreItems = computed(() => items.value.filter((i) => !i.tab))
const moreCurrent = computed(() => moreItems.value.some((i) => i.current))
const moreBadge = computed(() => moreItems.value.reduce((n, i) => n + i.badge, 0))

const accessibleName = (label: string, badge: number) => (badge ? `${label}, ${badge} unread run notification${badge === 1 ? "" : "s"}` : label)
const itemName = (item: NavItem) => accessibleName(item.label, item.badge)

const open = ref(false)
const moreButton = ref<HTMLButtonElement | null>(null)
const sheet = ref<HTMLElement | null>(null)
const route = useRoute()

watch(() => route.fullPath, () => (open.value = false))
watch(open, async (isOpen) => {
  if (isOpen) {
    await nextTick()
    sheet.value?.querySelector<HTMLElement>("a")?.focus()
  } else {
    moreButton.value?.focus()
  }
})
</script>

<template>
  <nav
    aria-label="Primary"
    class="fixed inset-x-0 bottom-0 z-30 grid grid-cols-5 bg-surface-raised pb-[env(safe-area-inset-bottom)]"
  >
    <RouterLink
      v-for="item in tabItems"
      :key="item.id"
      :to="item.to"
      :data-destination="item.id"
      :aria-label="itemName(item)"
      :aria-current="item.current ? 'page' : undefined"
      :class="['tab', { 'tab--current': item.current }]"
    >
      <span class="relative">
        <Icon :name="item.icon" size="md" />
        <NotificationDot v-if="item.badge" :count="item.badge" />
      </span>
      <span aria-hidden="true">{{ item.label }}</span>
    </RouterLink>

    <button
      ref="moreButton"
      type="button"
      aria-haspopup="dialog"
      aria-controls="more-sheet"
      :aria-expanded="open"
      :aria-label="accessibleName('More', moreBadge)"
      :data-current="moreCurrent ? 'true' : undefined"
      :class="['tab', { 'tab--current': moreCurrent || open }]"
      @click="open = !open"
    >
      <span class="relative">
        <Icon name="menu" size="md" />
        <NotificationDot v-if="moreBadge" :count="moreBadge" />
      </span>
      <span aria-hidden="true">More</span>
    </button>
  </nav>

  <template v-if="open">
    <div data-test="more-scrim" class="fixed inset-0 z-30 bg-scrim/50" aria-hidden="true" @click="open = false"></div>
    <div
      id="more-sheet"
      ref="sheet"
      role="dialog"
      aria-label="More"
      class="fixed inset-x-0 bottom-0 z-40 flex flex-col gap-1 rounded-t-[var(--radius-md)] bg-surface-raised p-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))]"
      @keydown.escape="open = false"
    >
      <RouterLink
        v-for="item in moreItems"
        :key="item.id"
        :to="item.to"
        :aria-label="itemName(item)"
        :aria-current="item.current ? 'page' : undefined"
        :class="['sheet-link', { 'sheet-link--current': item.current }]"
      >
        <span class="relative flex-none">
          <Icon :name="item.icon" size="md" />
          <NotificationDot v-if="item.badge" :count="item.badge" />
        </span>
        <span aria-hidden="true">{{ item.label }}</span>
      </RouterLink>
    </div>
  </template>
</template>

<style scoped>
.tab { @apply flex min-h-14 flex-col items-center justify-center gap-0.5 border-0 bg-transparent font-mono text-[11px] text-muted no-underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset; }
.tab--current { @apply text-primary; box-shadow: inset 0 2px 0 rgb(var(--accent-brand)); }
.sheet-link { @apply flex min-h-12 items-center gap-3 rounded-[var(--radius-md)] px-3 font-mono text-[length:var(--text-body)] text-secondary no-underline hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2; }
.sheet-link--current { @apply bg-surface-active text-primary; box-shadow: inset 2px 0 0 rgb(var(--accent-brand)); }
</style>
