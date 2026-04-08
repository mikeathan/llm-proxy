<script setup lang="ts">
import { computed } from 'vue';
import type { AutomationRun } from '../../types/dispatcher';

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
  <div class="flex flex-col h-full bg-gray-900/20">
    <div class="p-4 border-b border-gray-700/50 flex justify-between items-center">
      <h3 class="text-sm font-bold text-gray-400 uppercase tracking-widest">Workspace Activity</h3>
      <div v-if="loading" class="animate-spin h-4 w-4 border-2 border-blue-500 border-t-transparent rounded-full"></div>
    </div>

    <div class="flex-1 overflow-y-auto p-2">
      <div v-if="sortedHistory.length === 0" class="flex flex-col items-center justify-center h-40 text-gray-600">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-8 w-8 mb-2 opacity-20" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p class="text-xs italic">No activity recorded yet</p>
      </div>

      <div class="space-y-1">
        <div 
          v-for="run in sortedHistory" 
          :key="run.id"
          @click="emit('select-run', run)"
          class="group flex flex-col p-3 rounded-md hover:bg-gray-700/40 cursor-pointer border border-transparent hover:border-gray-600/30 transition-all"
        >
          <div class="flex items-center justify-between mb-1">
            <div class="flex items-center gap-2">
              <div :class="['w-2 h-2 rounded-full shadow-sm', run.error ? 'bg-red-500 shadow-red-900/50' : 'bg-green-500 shadow-green-900/50']"></div>
              <span class="text-[11px] font-bold text-gray-200 truncate max-w-[120px]">
                {{ run.automation_name }}
              </span>
            </div>
            <span class="text-[10px] font-mono text-gray-500 group-hover:text-gray-400">
              {{ formatTime(run.timestamp) }}
            </span>
          </div>

          <div class="flex items-center justify-between pl-4">
             <span class="text-[10px] text-gray-500">
               {{ formatDate(run.timestamp) }}
             </span>
             <span class="text-[9px] font-mono text-gray-600 group-hover:text-gray-500">
               {{ run.duration_ms }}ms
             </span>
          </div>
          
          <div v-if="run.error" class="mt-2 text-[10px] text-red-400/80 line-clamp-1 italic pl-4 border-l border-red-900/30">
            {{ run.error }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
