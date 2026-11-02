<script setup lang="ts">
import { computed } from "vue"
import type { NavItem } from "../../types/routes"
import Icon from "../icons/Icon.vue"
import NotificationDot from "../common/NotificationDot.vue"

// One destination in the left rail. Shared by the top-level entries and the destinations
// inside a group, so the two cannot drift apart; a nested one is indented under its heading.
const props = defineProps<{
  item: NavItem
  collapsed: boolean
  nested?: boolean
}>()

const accessibleName = computed(() =>
  props.item.badge ? `${props.item.label}, ${props.item.badge} unread run notification${props.item.badge === 1 ? "" : "s"}` : props.item.label,
)
</script>

<template>
  <RouterLink
    :to="item.to"
    :data-destination="item.id"
    :aria-label="accessibleName"
    :title="collapsed ? item.label : undefined"
    :aria-current="item.current ? 'page' : undefined"
    :class="[
      'flex h-8 items-center gap-2.5 rounded-[var(--radius-md)] px-2.5 font-mono text-[length:var(--text-small)] transition-colors duration-fast ease-standard focus-visible:outline-none focus-visible:ring-2',
      nested && !collapsed ? 'ml-3' : '',
      item.current ? 'bg-surface-active text-primary shadow-[inset_2px_0_0_rgb(var(--accent-brand))]' : 'text-muted hover:bg-surface-hover hover:text-primary',
    ]"
  >
    <span class="relative flex-none">
      <Icon :name="item.icon" size="sm" class-name="flex-none" />
      <NotificationDot v-if="item.badge" :count="item.badge" />
    </span>
    <span v-if="!collapsed" aria-hidden="true">{{ item.label }}</span>
  </RouterLink>
</template>
