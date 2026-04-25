<script setup lang="ts">
import {
  formatPercent,
  formatMemory,
  memPercent,
  formatTokenRate,
  gpuTempClass,
} from "../../../utils/formatters";
import type { SystemMetrics } from "../../../types/metrics";
import type { ActiveModel } from "../../../types/model";

const clampPercent = (v: number) => Math.min(Math.max(v, 0), 100);

defineProps<{
  activeModel: ActiveModel | undefined | null;
  metrics: SystemMetrics | null;
}>();
</script>

<template>
  <div class="metrics-expanded">
    <!-- Host Section -->
    <div class="metrics-section">
      <h4 class="section-title">Host Resources</h4>
      <div v-if="metrics" class="space-y-3">
        <div class="metric-block">
          <div class="metric-header">
            <span>CPU Load</span>
            <span class="metric-value">{{ formatPercent(metrics.load_percent) }}%</span>
          </div>
          <div class="progress-track">
            <div
              class="progress-bar progress-bar--blue"
              :style="{ width: `${clampPercent(metrics.load_percent ?? 0)}%` }"
            ></div>
          </div>
        </div>
        <div class="metric-block">
          <div class="metric-header">
            <span>Memory</span>
            <span class="metric-value">{{
              formatMemory(metrics.mem_used_mb, metrics.mem_total_mb)
            }}</span>
          </div>
          <div class="progress-track">
            <div
              class="progress-bar progress-bar--blue"
              :style="{
                width: `${clampPercent(memPercent(metrics.mem_used_mb, metrics.mem_total_mb))}%`,
              }"
            ></div>
          </div>
        </div>
      </div>
    </div>

    <!-- GPU Section -->
    <div v-if="metrics?.gpu" class="metrics-section border-t border-white/5 pt-3">
      <h4 class="section-title">GPU: {{ metrics.gpu.name || metrics.gpu.vendor }}</h4>
      <div class="space-y-3">
        <div class="metric-block">
          <div class="metric-header">
            <span>VRAM Utilization</span>
            <span class="metric-value">{{
              formatMemory(metrics.gpu.memory_used_mb, metrics.gpu.memory_total_mb)
            }}</span>
          </div>
          <div class="progress-track">
            <div
              class="progress-bar progress-bar--purple"
              :style="{
                width: `${clampPercent(metrics.gpu.memory_utilization_percent)}%`,
              }"
            ></div>
          </div>
        </div>
        <div class="flex justify-between text-[11px]">
          <span class="text-gray-400">Core Utilization</span>
          <span class="text-white font-bold">{{ metrics.gpu.utilization_percent }}%</span>
        </div>
        <div class="flex justify-between text-[11px]">
          <span class="text-gray-400">Temperature</span>
          <span :class="[gpuTempClass(metrics.gpu.temperature_c), 'font-bold']">
            {{ metrics.gpu.temperature_c }}°C
          </span>
        </div>
      </div>
    </div>

    <!-- Throughput Section -->
    <div class="metrics-section border-t border-white/5 pt-3">
      <div class="flex justify-between items-end">
        <div>
          <h4 class="section-title !mb-0">Throughput</h4>
          <p class="text-[10px] text-gray-500 italic">Live token rate</p>
        </div>
        <div class="text-right">
          <span class="text-2xl font-black text-white leading-none">
            {{ formatTokenRate(metrics?.llm_tokens_per_sec) }}
          </span>
          <span class="text-[10px] text-gray-400 ml-1 font-bold">T/S</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.metrics-expanded {
  @apply w-full space-y-4 animate-in fade-in zoom-in-95 duration-200;
}

.metrics-section {
  @apply w-full;
}

.section-title {
  @apply text-[10px] font-black text-gray-500 uppercase tracking-widest mb-3;
}

.metric-block {
  @apply space-y-1.5;
}

.metric-header {
  @apply flex justify-between text-[11px] font-medium text-gray-300;
}

.metric-value {
  @apply text-white font-bold;
}

.progress-track {
  @apply w-full bg-gray-900 rounded-full h-1.5 border border-white/5;
}

.progress-bar {
  @apply h-1.5 rounded-full transition-all duration-500;
}

.progress-bar--blue {
  @apply bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.3)];
}

.progress-bar--purple {
  @apply bg-purple-500 shadow-[0_0_8px_rgba(168,85,247,0.3)];
}
</style>
