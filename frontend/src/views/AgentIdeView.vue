<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useDispatcher } from '../composables/useDispatcher'
import type { Automation } from '../types/dispatcher'

const {
  automations,
  metrics,
  loading,
  error,
  fetchAutomations,
  fetchMetrics,
  triggerAutomation,
} = useDispatcher()

const selectedAutomation = ref<Automation | null>(null)
const triggering = ref(false)
const lastTriggerResult = ref<string | null>(null)

onMounted(() => {
  fetchAutomations()
  fetchMetrics()
})

const groupedByWorkspace = computed(() => {
  const groups: Record<string, Automation[]> = {}
  for (const auto of automations.value) {
    if (!groups[auto.workspace]) {
      groups[auto.workspace] = []
    }
    groups[auto.workspace]!.push(auto)
  }
  return groups
})

const selectAutomation = (auto: Automation) => {
  selectedAutomation.value = auto
  lastTriggerResult.value = null
}

const handleTrigger = async () => {
  if (!selectedAutomation.value) return
  triggering.value = true
  lastTriggerResult.value = null
  try {
    await triggerAutomation(selectedAutomation.value.workspace, selectedAutomation.value.name)
    lastTriggerResult.value = `Triggered ${selectedAutomation.value.name} successfully`
  } catch {
    lastTriggerResult.value = `Failed to trigger ${selectedAutomation.value.name}`
  } finally {
    triggering.value = false
  }
}
</script>

<template>
  <div class="h-[calc(100vh-8rem)] flex gap-4">
    <!-- Left Pane: Automation List -->
    <div class="w-72 flex flex-col bg-gray-800 rounded-lg overflow-hidden">
      <div class="p-4 border-b border-gray-700">
        <h2 class="font-semibold text-sm text-gray-300">Automations</h2>
        <p class="text-xs text-gray-500 mt-1">{{ automations.length }} total</p>
      </div>
      <div class="flex-1 overflow-y-auto">
        <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>
        <div v-else-if="error" class="p-4 text-red-400 text-sm">{{ error }}</div>
        <div v-else>
          <div v-for="(autos, workspace) in groupedByWorkspace" :key="workspace">
            <div class="px-4 py-2 bg-gray-750 text-xs font-semibold text-gray-400 uppercase">
              {{ workspace }}
            </div>
            <button
              v-for="auto in autos"
              :key="auto.id"
              @click="selectAutomation(auto)"
              :class="[
                'w-full px-4 py-2.5 text-left text-sm transition-colors',
                selectedAutomation?.id === auto.id
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
      </div>
    </div>

    <!-- Middle Pane: Automation Details + Output -->
    <div class="flex-1 flex flex-col bg-gray-800 rounded-lg overflow-hidden">
      <div v-if="!selectedAutomation" class="flex-1 flex items-center justify-center text-gray-500">
        Select an automation to view details
      </div>
      <template v-else>
        <div class="p-4 border-b border-gray-700">
          <h2 class="font-semibold text-lg">{{ selectedAutomation.name }}</h2>
          <p class="text-sm text-gray-400 mt-1">
            Workspace: <span class="text-gray-300">{{ selectedAutomation.workspace }}</span>
          </p>
          <div class="flex gap-4 mt-2 text-xs text-gray-500">
            <span>Trigger: {{ selectedAutomation.trigger }}</span>
            <span>Strategy: {{ selectedAutomation.strategy }}</span>
            <span>Task: {{ selectedAutomation.task_file }}</span>
          </div>
        </div>
        <div class="flex-1 p-4 overflow-y-auto">
          <div v-if="lastTriggerResult" :class="[
            'p-3 rounded text-sm mb-4',
            lastTriggerResult.includes('Failed') ? 'bg-red-900/50 text-red-300' : 'bg-green-900/50 text-green-300'
          ]">
            {{ lastTriggerResult }}
          </div>
          <div class="text-gray-400 text-sm">
            <p>Automation execution output will appear here after running.</p>
          </div>
        </div>
      </template>
    </div>

    <!-- Right Pane: Trigger + Metrics -->
    <div class="w-64 flex flex-col gap-4">
      <!-- Trigger Control -->
      <div class="bg-gray-800 rounded-lg p-4">
        <h3 class="font-semibold text-sm text-gray-300 mb-3">Trigger</h3>
        <button
          @click="handleTrigger"
          :disabled="!selectedAutomation || triggering"
          :class="[
            'w-full py-2 px-4 rounded font-medium text-sm transition-colors',
            !selectedAutomation || triggering
              ? 'bg-gray-700 text-gray-500 cursor-not-allowed'
              : 'bg-blue-600 hover:bg-blue-700 text-white'
          ]"
        >
          {{ triggering ? 'Triggering...' : 'Run Now' }}
        </button>
        <p v-if="!selectedAutomation" class="text-xs text-gray-500 mt-2">
          Select an automation first
        </p>
      </div>

      <!-- Dispatcher Metrics -->
      <div class="bg-gray-800 rounded-lg p-4 flex-1">
        <h3 class="font-semibold text-sm text-gray-300 mb-3">Metrics</h3>
        <div v-if="metrics" class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-gray-400">Total Runs</span>
            <span class="text-gray-200">{{ metrics.total_executions }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Successful</span>
            <span class="text-green-400">{{ metrics.successful }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Failed</span>
            <span class="text-red-400">{{ metrics.failed }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Skipped</span>
            <span class="text-yellow-400">{{ metrics.skipped }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Avg Latency</span>
            <span class="text-gray-200">{{ Math.round(metrics.total_latency_ms / Math.max(metrics.total_executions, 1)) }}ms</span>
          </div>
        </div>
        <div v-else class="text-gray-500 text-sm">No metrics available</div>
      </div>
    </div>
  </div>
</template>
