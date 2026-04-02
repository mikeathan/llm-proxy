<script setup lang="ts">
import type { Automation } from '../../types/dispatcher'

const props = defineProps<{
  groupedAutomations: Record<string, Automation[]>
  selectedAutomationId: string | undefined
}>()

const emit = defineEmits<{
  (e: 'select-automation', automation: Automation): void
}>()
</script>

<template>
  <div>
    <div v-for="(autos, workspace) in groupedAutomations" :key="workspace">
      <div class="px-4 py-2 bg-gray-750 text-xs font-semibold text-gray-400 uppercase">
        {{ workspace }}
      </div>
      <button
        v-for="auto in autos"
        :key="auto.id"
        @click="emit('select-automation', auto)"
        :class="[
          'w-full px-4 py-2.5 text-left text-sm transition-colors',
          selectedAutomationId === auto.id
            ? 'bg-blue-600 text-white'
            : 'text-gray-300 hover:bg-gray-700'
        ]"
      >
        <div class="font-medium">{{ auto.name }}</div>
        <div class="text-xs opacity-70 mt-0.5">
          {{ auto.trigger }} · {{ auto.strategy }}
        </div>
      </button>
    </div>
  </div>
</template>
