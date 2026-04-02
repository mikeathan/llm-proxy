<script setup lang="ts">
import type { DispatcherMetrics } from '../../types/dispatcher'

defineProps<{
  metrics: DispatcherMetrics | null
}>()
</script>

<template>
  <div class="bg-gray-800 rounded-lg p-4 flex-1">
    <h3 class="font-semibold text-sm text-gray-300 mb-3">System Metrics</h3>
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
</template>
