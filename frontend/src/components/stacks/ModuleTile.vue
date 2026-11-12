<script setup lang="ts">
import { computed, ref } from "vue"
import type { RouteLocationNamedRaw } from "vue-router"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import { MODULE_ABOUT_LABELS, MODULE_ABOUT_TITLE, MODULE_SETTINGS_LINK, goToModuleLabel, moduleElementId } from "../../constants/stacks"
import { moduleStatus } from "../../utils/stacks/modules"
import type { ModuleTone, StackModule } from "../../types/stacks"

// One module: its name, one line of what it does, the switch, and one quiet status line (state, what it sends, what it needs).
// A tile on a wide screen, a row on a phone: the same markup. A blocked module is dimmed; nothing here repeats the status as a tag.
// A module blocked by one it needs points at that module by name (`titles` maps ids to titles) and asks the page to take you there.
const props = defineProps<{ module: StackModule; busy: boolean; error?: string; settingsTo?: RouteLocationNamedRaw; titles?: Record<string, string> }>()
const aboutOpen = ref(false)
const emit = defineEmits<{ (e: "switch", value: boolean): void; (e: "goto", id: string): void }>()

const TONE_CLASS: Record<ModuleTone, string> = { success: "text-state-success", warning: "text-state-running", muted: "text-faint" }
const status = computed(() => moduleStatus(props.module))
const dimmed = computed(() => Boolean(props.module.blocked))
</script>

<template>
  <li
    :id="moduleElementId(module.id)"
    tabindex="-1"
    :data-test="`module-${module.id}`"
    :class="['flex min-w-0 flex-col gap-1.5 px-3 py-3 md:rounded-[var(--radius-md)] md:border md:border-hairline md:p-3.5', module.enabled ? 'md:bg-surface-raised' : 'md:bg-transparent', dimmed ? 'opacity-70' : '']"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="flex min-w-0 flex-col gap-0.5">
        <span class="font-medium text-primary">{{ module.title }}</span>
        <span class="text-[length:var(--text-small)] text-muted">{{ module.purpose }}</span>
      </div>
      <BaseToggle
        class="max-sm:-m-3.5 max-sm:p-3.5"
        :model-value="module.enabled"
        :label="module.title"
        hide-label
        :disabled="busy"
        @update:model-value="emit('switch', $event)"
      />
    </div>
    <span :class="['text-[length:var(--text-small)]', TONE_CLASS[status.tone]]" data-test="module-status">{{ status.text }}</span>
    <div v-if="module.blocked_by?.length" class="flex flex-wrap gap-x-3 gap-y-1">
      <button
        v-for="id in module.blocked_by"
        :key="id"
        type="button"
        class="text-left text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline max-sm:min-h-11"
        :data-test="`module-need-${id}`"
        @click="emit('goto', id)"
      >{{ goToModuleLabel(titles?.[id] ?? id) }}</button>
    </div>
    <Disclosure v-if="module.about" :title="MODULE_ABOUT_TITLE" :open="aboutOpen" data-test="module-about" @update:open="aboutOpen = $event">
      <dl class="m-0 flex flex-col gap-1.5 text-[length:var(--text-small)] text-secondary">
        <div v-for="(label, key) in MODULE_ABOUT_LABELS" :key="key" class="flex flex-col gap-0.5">
          <dt class="font-medium text-primary">{{ label }}</dt>
          <dd class="m-0">{{ module.about[key] }}</dd>
        </div>
      </dl>
      <RouterLink v-if="settingsTo && module.about.settings" :to="settingsTo" class="mt-1.5 inline-block text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline" data-test="module-settings">{{ MODULE_SETTINGS_LINK }}</RouterLink>
    </Disclosure>
    <span v-if="error" role="alert" class="text-[length:var(--text-small)] text-state-error" data-test="module-error">{{ error }}</span>
  </li>
</template>
