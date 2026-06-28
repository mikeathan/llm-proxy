<script setup lang="ts">
import { ref } from "vue";
import type { SystemMetrics } from "../../../types/metrics";
import type { ActiveModel } from "../../../types/model";
import MetricsMini from "../../AgentIde/system/MetricsMini.vue";
import MetricsExpanded from "../../AgentIde/system/MetricsExpanded.vue";

defineProps<{
  metrics: SystemMetrics | null;
  activeModel?: ActiveModel | null;
}>();

const isExpanded = ref(false);

const toggleExpand = () => {
  isExpanded.value = !isExpanded.value;
};
</script>

<template>
  <div 
    class="metrics-pulse" 
    :class="{ 'metrics-pulse--expanded': isExpanded }"
    @click="toggleExpand"
  >
    <transition name="fade-fast" mode="out-in">
      <component 
        :is="isExpanded ? MetricsExpanded : MetricsMini" 
        :metrics="metrics"
        :activeModel="activeModel"
      />
    </transition>
    
    <!-- Expansion Indicator -->
    <div class="expand-hint">
      <svg 
        xmlns="http://www.w3.org/2000/svg" 
        class="h-3 w-3 transition-transform duration-300" 
        :class="{ 'rotate-180': isExpanded }"
        fill="none" 
        viewBox="0 0 24 24" 
        stroke="currentColor"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
      </svg>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.metrics-pulse {
  @apply relative flex flex-col items-center gap-4 bg-gray-900/60 backdrop-blur-md border border-white/10 px-4 py-2 rounded-lg shadow-xl w-full cursor-pointer transition-all duration-300 hover:bg-gray-900/80 hover:border-white/20 select-none;
}

.metrics-pulse--expanded {
  @apply py-4 bg-gray-900/90;
}

.expand-hint {
  @apply absolute bottom-1 left-1/2 -translate-x-1/2 text-gray-600 opacity-40;
}

.metrics-pulse:hover .expand-hint {
  @apply opacity-100 text-gray-400;
}

/* Transitions */
.fade-fast-enter-active,
.fade-fast-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.fade-fast-enter-from {
  opacity: 0;
  transform: translateY(4px);
}

.fade-fast-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
