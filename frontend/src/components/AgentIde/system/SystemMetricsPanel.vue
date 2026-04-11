<script setup lang="ts">
import type { DispatcherMetrics } from '../../../types/dispatcher'

defineProps<{
  metrics: DispatcherMetrics | null
}>()
</script>

<template>
  <div class="metrics-panel">
    <h3 class="metrics-title">System Metrics</h3>
    <div v-if="metrics" class="metrics-list">
      <div class="metric-row">
        <span class="metric-label">Total Runs</span>
        <span class="metric-value">{{ metrics.total_executions }}</span>
      </div>
      <div class="metric-row">
        <span class="metric-label">Successful</span>
        <span class="metric-value metric-value--success">{{ metrics.successful }}</span>
      </div>
      <div class="metric-row">
        <span class="metric-label">Failed</span>
        <span class="metric-value metric-value--error">{{ metrics.failed }}</span>
      </div>
      <div class="metric-row">
        <span class="metric-label">Skipped</span>
        <span class="metric-value metric-value--warning">{{ metrics.skipped }}</span>
      </div>
      <div class="metric-row">
        <span class="metric-label">Avg Latency</span>
        <span class="metric-value">{{ Math.round(metrics.total_latency_ms / Math.max(metrics.total_executions, 1)) }}ms</span>
      </div>
    </div>
    <div v-else class="metrics-empty">No metrics available</div>
  </div>
</template>

<style scoped lang="postcss">
.metrics-panel {
  @apply bg-gray-800 rounded-lg p-4 flex-1;
}

.metrics-title {
  @apply font-semibold text-sm text-gray-300 mb-3;
}

.metrics-list {
  @apply space-y-2 text-sm;
}

.metric-row {
  @apply flex justify-between;
}

.metric-label {
  @apply text-gray-400;
}

.metric-value {
  @apply text-gray-200;
}

.metric-value--success {
  @apply text-green-400;
}

.metric-value--error {
  @apply text-red-400;
}

.metric-value--warning {
  @apply text-yellow-400;
}

.metrics-empty {
  @apply text-gray-500 text-sm;
}
</style>
