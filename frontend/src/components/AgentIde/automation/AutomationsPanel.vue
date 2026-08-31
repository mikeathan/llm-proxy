<script setup lang="ts">
import { ref } from 'vue'
import type { Automation } from '../../../types/dispatcher'
import InlineConfirm from '../../ui/InlineConfirm.vue'
import Icon from '../../icons/Icon.vue'

const props = defineProps<{
  groupedAutomations: Record<string, Automation[]>
  selectedAutomationId: string | undefined
}>()

const emit = defineEmits<{
  (e: 'select-automation', automation: Automation): void
  (e: 'edit-automation', automation: Automation): void
  (e: 'delete-automation', automation: Automation): void
}>()

const confirmingDeleteFor = ref<string | null>(null)

const confirmAndEmit = (auto: Automation) => {
  confirmingDeleteFor.value = null
  emit('delete-automation', auto)
}

const isWorkspaceBusy = (workspaceAutos: Automation[]) => {
  return workspaceAutos.some(a => a.is_running)
}

const isAutomationLocked = (auto: Automation, workspaceAutos: Automation[]) => {
  return isWorkspaceBusy(workspaceAutos) && !auto.is_running
}
</script>

<template>
  <div class="panel-container">
    <div v-for="(autos, workspace) in groupedAutomations" :key="workspace">
      <div class="workspace-header">
        {{ workspace }}
      </div>
      <div v-for="auto in autos" :key="auto.id">
        <div
          class="automation-row group"
          :class="{ 
            'automation-row--selected': selectedAutomationId === auto.id,
            'automation-row--disabled': isAutomationLocked(auto, autos)
          }"
        >
          <button
            @click="emit('select-automation', auto)"
            class="btn-automation-select"
            :disabled="isAutomationLocked(auto, autos)"
            :class="{ 'btn-automation-select--selected': selectedAutomationId === auto.id }"
          >
            <div class="automation-name">{{ auto.name }}</div>
            <div class="automation-meta">
              {{ auto.trigger }} · {{ auto.strategy }}
              <span v-if="auto.is_running" class="status-running">
                <span class="pulse-dot"></span>
                Running
              </span>
            </div>
          </button>
          
          <div class="row-actions">
            <button
              v-if="confirmingDeleteFor !== auto.id"
              @click.stop="confirmingDeleteFor = auto.id"
              class="btn-automation-action btn-automation-action--delete"
              :disabled="isWorkspaceBusy(autos)"
              title="Delete Automation"
            >
              <Icon name="trash" size="sm" />
            </button>
            <button
              @click.stop="emit('edit-automation', auto)"
              class="btn-automation-action btn-automation-action--edit"
              :disabled="isWorkspaceBusy(autos)"
              title="Edit Automation"
            >
              <Icon name="edit" size="sm" />
            </button>
          </div>
        </div>
        
        <div v-if="confirmingDeleteFor === auto.id" class="px-2">
          <InlineConfirm
            :message="`Delete '${auto.name}'?`"
            @confirm="confirmAndEmit(auto)"
            @cancel="confirmingDeleteFor = null"
            class="!mx-0 !my-1"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.panel-container {
  @apply divide-y divide-gray-800;
}

.workspace-header {
  @apply px-4 py-2 bg-gray-700 text-xs font-semibold text-gray-400 uppercase;
}

.automation-row {
  @apply relative flex items-center pr-2 transition-colors hover:bg-gray-700/50;
}

.automation-row--selected {
  @apply bg-blue-600 hover:bg-blue-600;
}
.automation-row--disabled {
  @apply opacity-60 grayscale-[0.3];
}
.automation-row--disabled .status-running {
  @apply opacity-100 grayscale-0;
}

.btn-automation-select {
  @apply flex-1 px-4 py-3 text-left transition-colors text-gray-300;
}

.btn-automation-select--selected {
  @apply text-white;
}

.automation-name {
  @apply font-medium text-sm;
}

.automation-meta {
  @apply text-xs opacity-70 mt-0.5 flex items-center gap-2;
}

.status-running {
  @apply flex items-center gap-1.5 text-[10px] text-green-400 font-bold uppercase tracking-wider ml-auto;
}

.pulse-dot {
  @apply w-1.5 h-1.5 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)];
}

.row-actions {
  @apply flex items-center gap-1;
}

.btn-automation-action {
  @apply p-2 text-gray-400 transition-colors opacity-0 group-hover:opacity-100;
}

.btn-automation-action--delete {
  @apply hover:text-red-400;
}

.btn-automation-action--edit {
  @apply hover:text-blue-400;
}

.automation-row--selected .btn-automation-action {
  @apply text-white/70 hover:text-white opacity-100;
}
</style>
