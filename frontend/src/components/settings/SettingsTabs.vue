<script setup lang="ts">
import { computed } from "vue"
import { toSettings } from "../../router/routes"
import { groupOfSection } from "../../domain/settings"
import type { SettingsTab, SettingsTabGroup } from "../../types/admin"
import ScrollStrip from "../common/layout/ScrollStrip.vue"

// The Settings tabs (plan frontend-redesign-v2, V3): one tab per group, and for a
// group that holds several sections (the providers, Security/Guardrails, the
// connectors) a second row of them. Every section is still its own route, so the
// links are plain navigation — deep links and the browser's back button keep
// working — and a group's tab opens its first section.
const props = defineProps<{ groups: SettingsTabGroup[]; active: SettingsTab; labelOf: (tab: SettingsTab) => string }>()

const activeGroup = computed(() => groupOfSection(props.groups, props.active))
const firstSection = (group: SettingsTabGroup) => group.sections[0] as SettingsTab
</script>

<template>
  <div class="flex flex-col gap-1">
    <ScrollStrip label="Settings" :current="active" class="settings-tabs">
      <RouterLink
        v-for="group in groups"
        :key="group.id"
        :to="toSettings(firstSection(group))"
        :aria-current="activeGroup?.id === group.id ? 'page' : undefined"
        class="settings-tab"
      >{{ group.label }}</RouterLink>
    </ScrollStrip>

    <ScrollStrip v-if="activeGroup && activeGroup.sections.length > 1" :label="`${activeGroup.label} sections`" :current="active" class="settings-chips">
      <RouterLink
        v-for="section in activeGroup.sections"
        :key="section"
        :to="toSettings(section)"
        :aria-current="active === section ? 'page' : undefined"
        class="settings-chip"
      >{{ labelOf(section) }}</RouterLink>
    </ScrollStrip>
  </div>
</template>

<style scoped lang="postcss">
.settings-tab {
  @apply inline-flex h-11 flex-none items-center border-b-2 border-transparent px-3.5 text-muted no-underline hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}
.settings-tab[aria-current="page"] {
  @apply border-accent-brand font-medium text-primary;
}
.settings-chip {
  @apply inline-flex h-8 flex-none items-center rounded-[var(--radius-md)] px-3 text-[length:var(--text-small)] text-muted no-underline hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}
.settings-chip[aria-current="page"] {
  @apply bg-surface-active text-primary;
}
.settings-chips {
  @apply pt-2;
}
</style>
