<script setup lang="ts">
import { computed } from "vue";
import type { AutomationRun } from "../../../types/dispatcher";

const props = defineProps<{
  history: AutomationRun[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  (e: "select-run", run: AutomationRun): void;
}>();

// Flat list of runs, sorted by most recent first
const sortedHistory = computed(() => {
  return [...props.history].sort(
    (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
  );
});

import { formatTime, formatDate } from "../../../utils/time";


const getStatusClass = (run: AutomationRun) => {
  return run.error ? "border-l-red-500" : "border-l-green-500";
};

const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remSecs = secs % 60;
  if (mins < 60) return `${mins}m ${remSecs}s`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m ${remSecs}s`;
};
</script>

<template>
  <div class="activity-shell">
    <div class="activity-header">
      <div class="header-left">
        <h3 class="header-title">Live Pulse</h3>
        <span class="pulse-count">{{ sortedHistory.length }} total logs</span>
      </div>
      <div v-if="loading" class="loader-spinner"></div>
    </div>

    <div class="activity-content">
      <div v-if="sortedHistory.length === 0" class="empty-activity">
        <p class="empty-text">No execution history</p>
      </div>

      <div class="activity-list">
        <div
          v-for="run in sortedHistory"
          :key="run.id"
          @click="emit('select-run', run)"
          class="automation-card"
          :class="getStatusClass(run)"
        >
          <div class="card-main">
            <span class="auto-name">{{ run.automation_name }}</span>
            <span class="latest-time">{{ formatTime(run.timestamp) }}</span>
          </div>
          
          <div class="card-footer">
            <div class="meta-item">
              <span class="meta-label">ID:</span>
              <span class="meta-val">{{ run.id.slice(-6) }}</span>
            </div>
            <div class="meta-item">
              <span class="meta-label">Time:</span>
              <span class="meta-val">{{ formatDuration(run.duration_ms) }}</span>
            </div>
            <span class="latest-date">{{ formatDate(run.timestamp) }}</span>
          </div>

          <!-- Simple status indicator if it was a failure -->
          <div v-if="run.error" class="fail-tag">Failed</div>
          <div v-else class="success-tag">Success</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.activity-shell {
  @apply flex flex-col h-full bg-gray-900/40;
}

.activity-header {
  @apply p-4 border-b border-gray-800 flex justify-between items-center bg-gray-900/60;
}

.header-left {
  @apply flex items-center gap-3;
}

.header-title {
  @apply text-[10px] font-bold text-gray-400 uppercase tracking-widest;
}

.pulse-count {
  @apply text-[8px] bg-gray-800 text-gray-500 px-1.5 py-0.5 rounded font-mono;
}

.loader-spinner {
  @apply animate-spin h-3 w-3 border-2 border-blue-500 border-t-transparent rounded-full;
}

.activity-content {
  @apply flex-1 overflow-y-auto p-4;
}

.empty-activity {
  @apply flex items-center justify-center py-10 opacity-30 italic text-[10px];
}

.activity-list {
  @apply space-y-4;
}

.automation-card {
  @apply p-4 border-l-2 rounded-lg bg-gray-800/10 cursor-pointer hover:bg-gray-800/40 transition-all border-gray-800/50 hover:border-gray-600;
}

.card-main {
  @apply flex justify-between items-start mb-3;
}

.auto-name {
  @apply text-xs font-black text-gray-100 uppercase tracking-tight;
}

.latest-time {
  @apply text-[10px] font-mono text-gray-500;
}

.card-footer {
  @apply flex justify-between items-center;
}

.meta-item {
  @apply flex items-center gap-1.5;
}

.meta-label {
  @apply text-[9px] text-gray-600 uppercase font-bold;
}

.meta-val {
  @apply text-[9px] text-gray-500 font-mono;
}

.latest-date {
  @apply text-[9px] text-gray-500 opacity-60;
}

.fail-tag {
  @apply mt-3 text-[8px] bg-red-900/20 text-red-500 px-2 py-0.5 rounded-full font-black uppercase tracking-widest border border-red-500/20 inline-block;
}

.success-tag {
  @apply mt-3 text-[8px] bg-green-900/20 text-green-500 px-2 py-0.5 rounded-full font-black uppercase tracking-widest border border-green-500/20 inline-block;
}
</style>
