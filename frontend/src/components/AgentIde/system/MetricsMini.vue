<script setup lang="ts">
import { computed } from "vue";
import { formatPercent, formatTokenRate } from "../../../utils/formatters";
import type { SystemMetrics } from "../../../types/metrics";

const props = defineProps<{
  metrics: SystemMetrics | null;
}>();

const cpuLoad = computed(() => props.metrics?.load_percent ?? 0);
const gpuLoad = computed(() => props.metrics?.gpu?.utilization_percent ?? 0);
const throughput = computed(() => props.metrics?.llm_tokens_per_sec ?? 0);

const getLoadColor = (val: number) => {
  if (val > 80) return "text-red-400";
  if (val > 50) return "text-yellow-400";
  return "text-blue-400";
};
</script>

<template>
  <div class="metrics-mini">
    <!-- CPU -->
    <div class="metric-item" title="CPU Load">
      <span class="metric-icon">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
        </svg>
      </span>
      <span :class="['metric-val', getLoadColor(cpuLoad)]">{{ formatPercent(cpuLoad) }}%</span>
    </div>

    <!-- GPU -->
    <div v-if="metrics?.gpu" class="metric-item" title="GPU Core Utilization">
      <span class="metric-icon">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 1.1.9 2 2 2h12a2 2 0 002-2V7a2 2 0 00-2-2H6a2 2 0 00-2 2zm4-4h8" />
        </svg>
      </span>
      <span :class="['metric-val', getLoadColor(gpuLoad)]">{{ formatPercent(gpuLoad) }}%</span>
    </div>

    <!-- Throughput -->
    <div class="metric-item throughput" title="LLM Throughput">
      <span class="metric-icon">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </span>
      <span class="metric-val text-white font-mono">{{ formatTokenRate(throughput) }}</span>
      <span class="metric-unit">t/s</span>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.metrics-mini {
  @apply flex items-center gap-4 w-full;
}

.metric-item {
  @apply flex items-center gap-2;
}

.metric-icon {
  @apply text-gray-500 shrink-0;
}

.metric-val {
  @apply text-[12px] font-bold tracking-tight font-mono w-[42px] text-right;
}

.throughput {
  @apply pl-4 border-l border-white/10 flex-1 justify-end;
}

.throughput .metric-val {
  @apply w-auto;
}

.metric-unit {
  @apply text-[10px] text-gray-500 font-medium uppercase shrink-0;
}
</style>
