<template>
  <div class="logs-container">
    <div class="logs-header">
      <h2 class="logs-title">
        Model Process Logs
        <span v-if="logs?.running" class="status-indicator">
          <span class="status-ping"></span>
          <span class="status-dot"></span>
        </span>
      </h2>
      <div class="logs-status">
        <span v-if="logs?.running" class="status-text-running">Model: <span class="status-model-name">{{ logs?.name }}</span></span>
        <span v-else class="status-text-stopped">Process stopped</span>
      </div>
    </div>

    <div class="logs-content">
      {{ logs?.logs || 'No logs available.' }}
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  logs: any
}>()
</script>

<style scoped lang="postcss">
.logs-container {
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 p-0 flex flex-col h-[700px] overflow-hidden;
}
.logs-header {
  @apply p-4 border-b border-gray-700 flex justify-between items-center bg-gray-800 z-10;
}
.logs-title {
  @apply text-lg font-semibold text-white flex items-center gap-2;
}
.status-indicator {
  @apply flex h-2 w-2 relative;
}
.status-ping {
  @apply animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75;
}
.status-dot {
  @apply relative inline-flex rounded-full h-2 w-2 bg-green-500;
}
.logs-status {
  @apply flex gap-3 text-sm;
}
.status-text-running {
  @apply text-gray-400;
}
.status-model-name {
  @apply text-white;
}
.status-text-stopped {
  @apply text-yellow-500;
}
.logs-content {
  @apply flex-1 bg-[#1e1e1e] p-4 overflow-y-auto font-mono text-xs text-gray-300 leading-relaxed whitespace-pre-wrap select-text break-words;
}
</style>
