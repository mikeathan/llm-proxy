<script setup lang="ts">
import { toRef } from 'vue'
import { useAssistantMemory } from '../../../composables/memory/useAssistantMemory'
import { useMemoryDefaults } from '../../../composables/memory/useMemoryDefaults'
import InheritField from '../../common/forms/InheritField.vue'
import type { MemoryMode } from '../../../types/automation'

// Whether this workspace's assistant chats get hot memory: the global default
// from Settings, or an override for this workspace only. Saves on change.
const props = defineProps<{ workspaceId: string }>()

const { assistantDefaultOn } = useMemoryDefaults()
const { mode, loading, saving, error, save } = useAssistantMemory(toRef(props, 'workspaceId'))
</script>

<template>
  <div class="flex flex-col gap-1.5">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <span class="text-[length:var(--text-small)] font-medium text-secondary">Assistant memory in this workspace</span>
      <InheritField
        :model-value="mode"
        :default-on="assistantDefaultOn"
        label="Assistant memory"
        :aria-busy="loading || saving"
        @update:model-value="save($event as MemoryMode)"
      />
    </div>
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <p v-else class="m-0 text-[length:var(--text-small)] text-faint">
      Chats here get your always-on facts at the start of each run. The default is set in Settings.
    </p>
  </div>
</template>
