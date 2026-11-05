<script setup lang="ts">
import { computed } from "vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import { moduleStatus } from "../../utils/stacks/modules"
import type { ModuleTone, StackModule } from "../../types/stacks"

// One module: its name, one line of what it does, the switch, and one quiet status line (state, what it sends, what it needs).
// A tile on a wide screen, a row on a phone: the same markup. A blocked module is dimmed; nothing here repeats the status as a tag.
const props = defineProps<{ module: StackModule; busy: boolean; error?: string }>()
const emit = defineEmits<{ (e: "switch", value: boolean): void }>()

const TONE_CLASS: Record<ModuleTone, string> = { success: "text-state-success", warning: "text-state-running", muted: "text-faint" }
const status = computed(() => moduleStatus(props.module))
const dimmed = computed(() => Boolean(props.module.blocked))
</script>

<template>
  <li
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
    <span v-if="error" role="alert" class="text-[length:var(--text-small)] text-state-error" data-test="module-error">{{ error }}</span>
  </li>
</template>
