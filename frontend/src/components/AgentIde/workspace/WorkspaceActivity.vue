<script setup lang="ts">
import { computed } from 'vue';
import type { AutomationRun } from '../../../types/dispatcher';

const props = defineProps<{
  history: AutomationRun[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  (e: 'select-run', run: AutomationRun): void;
}>();

const sortedHistory = computed(() => {
  return [...props.history].sort((a, b) => 
    new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
  );
});

const formatTime = (ts: string) => {
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
};

const formatDate = (ts: string) => {
  return new Date(ts).toLocaleDateString([], { month: 'short', day: 'numeric' });
};
</script>

<template>
  <div class="activity-shell">
    <div class="activity-header">
      <h3 class="header-title">Workspace Activity</h3>
      <div v-if="loading" class="loader-spinner"></div>
    </div>

    <div class="activity-content">
      <div v-if="sortedHistory.length === 0" class="empty-activity">
        <svg xmlns="http://www.w3.org/2000/svg" class="empty-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p class="empty-text">No activity recorded yet</p>
      </div>

      <div class="activity-list">
        <div 
          v-for="run in sortedHistory" 
          :key="run.id"
          @click="emit('select-run', run)"
          class="activity-item group"
        >
          <div class="item-top-row">
            <div class="item-status-group">
              <div 
                class="status-dot shadow-sm" 
                :class="run.error ? 'status-dot--error' : 'status-dot--success'"
              ></div>
              <span class="item-name">
                {{ run.automation_name }}
              </span>
            </div>
            <span class="item-time">
              {{ formatTime(run.timestamp) }}
            </span>
          </div>

          <div class="item-bottom-row">
             <span class="item-date">
               {{ formatDate(run.timestamp) }}
             </span>
             <span class="item-duration">
               {{ run.duration_ms }}ms
             </span>
          </div>
          
          <div v-if="run.error" class="item-error-msg">
            {{ run.error }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.activity-shell {
  @apply flex flex-col h-full bg-gray-900/20;
}

.activity-header {
  @apply p-4 border-b border-gray-700/50 flex justify-between items-center;
}

.header-title {
  @apply text-sm font-bold text-gray-400 uppercase tracking-widest;
}

.loader-spinner {
  @apply animate-spin h-4 w-4 border-2 border-blue-500 border-t-transparent rounded-full;
}

.activity-content {
  @apply flex-1 overflow-y-auto p-2;
}

.empty-activity {
  @apply flex flex-col items-center justify-center h-40 text-gray-600;
}

.empty-icon {
  @apply h-8 w-8 mb-2 opacity-20;
}

.empty-text {
  @apply text-xs italic;
}

.activity-list {
  @apply space-y-1;
}

.activity-item {
  @apply flex flex-col p-3 rounded-md hover:bg-gray-700/40 cursor-pointer border border-transparent 
         hover:border-gray-600/30 transition-all;
}

.item-top-row {
  @apply flex items-center justify-between mb-1;
}

.item-status-group {
  @apply flex items-center gap-2;
}

.status-dot {
  @apply w-2 h-2 rounded-full;
}

.status-dot--error {
  @apply bg-red-500 shadow-red-900/50;
}

.status-dot--success {
  @apply bg-green-500 shadow-green-900/50;
}

.item-name {
  @apply text-[11px] font-bold text-gray-200 truncate max-w-[120px];
}

.item-time {
  @apply text-[10px] font-mono text-gray-500 group-hover:text-gray-400;
}

.item-bottom-row {
  @apply flex items-center justify-between pl-4;
}

.item-date {
  @apply text-[10px] text-gray-500;
}

.item-duration {
  @apply text-[9px] font-mono text-gray-600 group-hover:text-gray-500;
}

.item-error-msg {
  @apply mt-2 text-[10px] text-red-400/80 line-clamp-1 italic pl-4 border-l border-red-900/30;
}
</style>
