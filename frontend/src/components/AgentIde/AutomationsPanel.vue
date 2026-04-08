<script setup lang="ts">
import { ref } from 'vue'
import type { Automation } from '../../types/dispatcher'
import InlineConfirm from '../ui/InlineConfirm.vue'

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
</script>

<template>
  <div>
    <div v-for="(autos, workspace) in groupedAutomations" :key="workspace">
      <div class="px-4 py-2 bg-gray-750 text-xs font-semibold text-gray-400 uppercase">
        {{ workspace }}
      </div>
      <div v-for="auto in autos" :key="auto.id">
        <div
          class="group relative flex items-center pr-2"
          :class="selectedAutomationId === auto.id ? 'bg-blue-600' : 'hover:bg-gray-700'"
        >
          <button
            @click="emit('select-automation', auto)"
            :class="[
              'flex-1 px-4 py-2.5 text-left text-sm transition-colors',
              selectedAutomationId === auto.id ? 'text-white' : 'text-gray-300'
            ]"
          >
            <div class="font-medium">{{ auto.name }}</div>
            <div class="text-xs opacity-70 mt-0.5">
              {{ auto.trigger }} · {{ auto.strategy }}
            </div>
          </button>
          <button
            v-if="confirmingDeleteFor !== auto.id"
            @click.stop="confirmingDeleteFor = auto.id"
            class="p-2 text-gray-400 hover:text-red-400 transition-colors opacity-0 group-hover:opacity-100"
            title="Delete Automation"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </button>
          <button
            @click.stop="emit('edit-automation', auto)"
            class="p-2 text-gray-400 hover:text-blue-400 transition-colors opacity-0 group-hover:opacity-100"
            title="Edit Automation"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
          </button>
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
