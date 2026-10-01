<script setup lang="ts">
import { useRouter } from "vue-router"
import { toSettings } from "../../router/routes"
import type { SettingsGroup, SettingsTab } from "../../types/admin"
import MicroLabel from "../common/display/MicroLabel.vue"
import FormField from "../common/forms/FormField.vue"

// Settings categories as links (every section is a route, plan D18), grouped;
// below `lg` the list becomes a category picker so the form keeps the width.
defineProps<{ groups: SettingsGroup[]; active: SettingsTab; labelOf: (tab: SettingsTab) => string }>()

const router = useRouter()
const open = (tab: string) => void router.push(toSettings(tab as SettingsTab))
</script>

<template>
  <nav aria-label="Settings categories" class="sticky top-4 hidden flex-col gap-4 lg:flex">
    <div v-for="group in groups" :key="group.name" class="flex flex-col gap-0.5">
      <MicroLabel class="mb-1 px-2.5">{{ group.name }}</MicroLabel>
      <RouterLink
        v-for="tab in group.tabs"
        :key="tab"
        :to="toSettings(tab)"
        :aria-current="active === tab ? 'page' : undefined"
        class="settings-link"
      >{{ labelOf(tab) }}</RouterLink>
    </div>
  </nav>

  <div class="lg:hidden">
    <FormField label="Category">
      <template #default="{ id }">
        <select :id="id" :value="active" class="form-control" @change="open(($event.target as HTMLSelectElement).value)">
          <optgroup v-for="group in groups" :key="group.name" :label="group.name">
            <option v-for="tab in group.tabs" :key="tab" :value="tab">{{ labelOf(tab) }}</option>
          </optgroup>
        </select>
      </template>
    </FormField>
  </div>
</template>

<style scoped lang="postcss">
.settings-link {
  @apply rounded-[var(--radius-md)] px-2.5 py-1.5 text-muted no-underline hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}
.settings-link[aria-current="page"] {
  @apply bg-surface-active text-primary;
  box-shadow: inset 2px 0 0 rgb(var(--accent-brand));
}
</style>
