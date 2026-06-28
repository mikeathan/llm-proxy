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

import { formatDate } from "../../../utils/format/time";


const getStatusClass = (run: AutomationRun) => {
  if (run.error) return run.recording_ref ? "border-l-red-500 recording-run" : "border-l-red-500";
  return run.recording_ref ? "border-l-green-500 recording-run" : "border-l-green-500";
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
          <div class="card-row card-row--top">
            <span class="auto-name">
              {{ run.automation_name }}
              <span v-if="run.recording_ref" class="rec-badge">REC</span>
            </span>
            <span class="auto-date">{{ formatDate(run.timestamp) }}</span>
          </div>

          <div class="card-row card-row--model">
            <span class="auto-model" :title="run.model || ''">{{ run.model || "Default" }}</span>
          </div>

          <div class="card-row card-row--bottom">
            <div class="meta-group">
              <span class="meta-val">{{ run.id.slice(-6) }}</span>
              <span class="meta-sep">·</span>
              <span class="meta-val">{{ formatDuration(run.duration_ms) }}</span>
            </div>
            <span v-if="run.error" class="fail-tag">Failed</span>
            <span v-else class="success-tag">Success</span>
          </div>
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
.automation-card.recording-run {
  @apply bg-amber-900/10 border-l-amber-600/50 hover:bg-amber-900/20;
}

.rec-badge {
  @apply text-[8px] font-bold text-amber-400 bg-amber-900/40 px-1.5 py-0.5 rounded ml-1.5 align-middle;
}

.card-row {
  @apply flex justify-between items-center;
}
.card-row--top {
  @apply mb-1;
}
.card-row--model {
  @apply mb-2;
}
.card-row--bottom {
  @apply gap-2;
}

.auto-name {
  @apply text-xs font-black text-gray-100 uppercase tracking-tight;
}

.auto-date {
  @apply text-[9px] text-gray-500 shrink-0;
}

.auto-model {
  @apply text-[10px] text-gray-600 truncate;
}

.meta-group {
  @apply flex items-center gap-1.5 min-w-0;
}

.meta-sep {
  @apply text-gray-700 text-[8px];
}

.meta-val {
  @apply text-[9px] text-gray-500 font-mono;
}

.fail-tag {
  @apply text-[8px] bg-red-900/20 text-red-500 px-2 py-0.5 rounded-full font-black uppercase tracking-widest border border-red-500/20 inline-block shrink-0;
}

.success-tag {
  @apply text-[8px] bg-green-900/20 text-green-500 px-2 py-0.5 rounded-full font-black uppercase tracking-widest border border-green-500/20 inline-block shrink-0;
}
</style>
