<template>
  <div class="system-status-container">
    <!-- Active Model Status -->
    <div class="status-card">
      <h3 class="card-title">Active Model</h3>
      <div v-if="activeModel" class="active-model-info">
        <div class="model-header">
          <span :class="['model-indicator', activeModel.ready ? 'indicator-ready' : 'indicator-loading']"></span>
          <span class="model-name" :title="activeModel.name">{{ activeModel.name }}</span>
        </div>
        <div class="model-endpoint">{{ activeModel.endpoint }}</div>
        <button @click="$emit('stopModel')" class="btn-stop">Stop Model</button>
      </div>
      <div v-else class="empty-state">No active model running</div>
    </div>

    <!-- Host Metrics -->
    <div class="status-card">
      <h3 class="card-title">Host Metrics</h3>
      <div v-if="metrics" class="metrics-list">
        <div>
          <div class="metric-row">
            <span>CPU Load</span>
            <span class="metric-value">{{ formatPercent(metrics.load_percent) }}%</span>
          </div>
          <div class="progress-track">
            <div class="progress-bar-blue" :style="{ width: `${clampPercent(metrics.load_percent ?? 0)}%` }"></div>
          </div>
        </div>
        <div>
          <div class="metric-row">
            <span>Memory</span>
            <span class="metric-value">{{ formatMemory(metrics.mem_used_mb, metrics.mem_total_mb) }}</span>
          </div>
          <div class="progress-track">
            <div class="progress-bar-blue" :style="{ width: `${clampPercent(memPercent(metrics.mem_used_mb, metrics.mem_total_mb))}%` }"></div>
          </div>
        </div>
      </div>
    </div>

    <!-- GPU Metrics -->
    <div class="status-card">
      <h3 class="card-title">GPU Status</h3>
      <div v-if="metrics?.gpu" class="metrics-list">
        <div>
          <div class="metric-row">
            <span>VRAM ({{ metrics.gpu.name || metrics.gpu.vendor }})</span>
            <span class="metric-value">{{ formatMemory(metrics.gpu.memory_used_mb, metrics.gpu.memory_total_mb) }}</span>
          </div>
          <div class="progress-track">
            <div class="progress-bar-purple" :style="{ width: `${clampPercent(metrics.gpu.memory_utilization_percent)}%` }"></div>
          </div>
        </div>
        <div class="metric-row-margin">
          <span>Core Utilization</span>
          <span class="metric-value">{{ metrics.gpu.utilization_percent }}%</span>
        </div>
        <div class="metric-row-plain">
          <span>Temperature</span>
          <span :class="gpuTempClass(metrics.gpu.temperature_c)">{{ metrics.gpu.temperature_c }}°C</span>
        </div>
      </div>
      <div v-else class="empty-state-margin">
        <span v-if="metrics?.gpu_error">{{ metrics.gpu_error }}</span>
        <span v-else>No GPU detected</span>
      </div>
    </div>

    <!-- LLM Throughput -->
    <div class="throughput-card">
      <h3 class="throughput-title">Throughput</h3>
      <div class="throughput-value">
        {{ formatTokenRate(metrics?.llm_tokens_per_sec) }}
      </div>
      <div class="throughput-label">tokens / sec</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { formatPercent, formatMemory, memPercent, formatTokenRate, gpuTempClass } from '../utils/formatters'
import type { SystemMetrics } from '../types/metrics'
import type { ActiveModel } from '../types/model'

const clampPercent = (v: number) => Math.min(Math.max(v, 0), 100)

defineProps<{
  activeModel: ActiveModel | undefined | null
  metrics: SystemMetrics | null
}>()

defineEmits<{
  (e: 'stopModel'): void
}>()
</script>

<style scoped lang="postcss">
.system-status-container {
  @apply grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4;
}
.status-card {
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col;
}
.card-title {
  @apply text-sm font-medium text-gray-400 uppercase tracking-wider mb-2;
}
.active-model-info {
  @apply flex flex-col gap-2 h-full;
}
.model-header {
  @apply flex items-center gap-2;
}
.model-indicator {
  @apply w-3 h-3 rounded-full;
}
.indicator-ready {
  @apply bg-green-500;
}
.indicator-loading {
  @apply bg-yellow-500 animate-pulse;
}
.model-name {
  @apply font-bold text-white text-lg truncate;
}
.model-endpoint {
  @apply text-xs text-gray-400;
}
.btn-stop {
  @apply mt-auto px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-xs font-medium rounded self-start transition-colors;
}
.empty-state {
  @apply text-gray-500 italic mt-2;
}
.metrics-list {
  @apply space-y-3;
}
.metric-row {
  @apply flex justify-between text-xs mb-1;
}
.metric-value {
  @apply text-white;
}
.progress-track {
  @apply w-full bg-gray-700 rounded-full h-1.5;
}
.progress-bar-blue {
  @apply bg-blue-500 h-1.5 rounded-full;
}
.progress-bar-purple {
  @apply bg-purple-500 h-1.5 rounded-full;
}
.metric-row-margin {
  @apply flex justify-between text-xs mt-2;
}
.metric-row-plain {
  @apply flex justify-between text-xs;
}
.empty-state-margin {
  @apply text-gray-500 text-sm mt-2;
}
.throughput-card {
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col justify-center items-center;
}
.throughput-title {
  @apply text-sm font-medium text-gray-400 uppercase tracking-wider mb-2 self-start w-full;
}
.throughput-value {
  @apply text-4xl font-bold text-white tracking-tight;
}
.throughput-label {
  @apply text-xs text-gray-500 mt-1;
}
</style>
