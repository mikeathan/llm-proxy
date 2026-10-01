<script setup lang="ts">
import type { DispatcherMetrics } from '../../../types/dispatcher'
import { formatDuration } from '../../../utils/format/time'

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
        <span class="metric-value">{{ formatDuration(metrics.total_latency_ms / Math.max(metrics.total_executions, 1)) }}</span>
      </div>
    </div>
    <div v-else class="metrics-empty">No metrics available</div>
  </div>
</template>

<style scoped lang="postcss">
.metrics-panel {
  @apply bg-surface border border-hairline rounded-[var(--radius-sm)] p-4 flex-1;
}

.metrics-title {
  @apply font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted mb-3;
}

.metrics-list {
  @apply space-y-2 text-sm;
}

.metric-row {
  @apply flex justify-between;
}

.metric-label {
  @apply text-muted;
}

.metric-value {
  @apply text-primary;
}

.metric-value--success {
  @apply text-state-success;
}

.metric-value--error {
  @apply text-state-error;
}

.metric-value--warning {
  @apply text-state-running;
}

.metrics-empty {
  @apply text-faint text-sm;
}
</style>
