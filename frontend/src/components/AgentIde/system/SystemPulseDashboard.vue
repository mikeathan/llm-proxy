<script setup lang="ts">
import { ref, computed } from "vue";
import type { AutomationRun } from "../../../types/dispatcher";
import { FOLDER_ICON } from "../../../constants/icons";

const props = defineProps<{
  selectedWorkspace?: string | null;
  loading?: boolean;
  workspaceHistory: AutomationRun[];
}>();

const emit = defineEmits<{
  (e: "select-run", run: AutomationRun): void;
  (e: "clear-workspace"): void;
}>();

const filterMode = ref<"all" | "errors">("all");

const filteredHistory = computed(() => {
  if (filterMode.value === "errors") {
    return props.workspaceHistory.filter((run) => run.error);
  }
  return props.workspaceHistory;
});
</script>

<template>
  <div class="dashboard-shell">
    <div class="dashboard-inner">
      <header class="header-section">
        <div class="header-row">
          <div class="header-left">
            <h1 class="main-title">System Pulse</h1>
            <p class="dashboard-subtitle">
              <span v-if="selectedWorkspace">
                Activity log for
                <span class="workspace-inline">
                  <span class="workspace-icon">{{ FOLDER_ICON }}</span>
                  <span class="workspace-name">{{ selectedWorkspace }}</span>
                  <button @click="emit('clear-workspace')" class="workspace-clear" title="Show all workspaces">
                    ✕
                  </button>
                </span>
              </span>
              <span v-else>Global activity log for all workspaces</span>
            </p>
          </div>
          <div v-if="loading" class="loader-spinner"></div>
        </div>

        <div class="filter-bar">
          <button
            @click="filterMode = 'all'"
            class="btn-filter"
            :class="{ 'btn-filter--active': filterMode === 'all' }"
          >
            All
          </button>
          <button
            @click="filterMode = 'errors'"
            class="btn-filter"
            :class="{ 'btn-filter--active': filterMode === 'errors' }"
          >
            Errors
          </button>
        </div>
      </header>

      <div v-if="filteredHistory.length === 0" class="empty-state">
        <p class="empty-state-text">
          <span v-if="filterMode === 'errors'"
            >No errors found in this operational stream.</span
          >
          <span v-else
            >No operational history found. Your fleet is waiting for
            instructions.</span
          >
        </p>
      </div>

      <div v-else class="timeline-stream">
        <!-- Chronological Rail -->
        <div class="timeline-rail"></div>

        <div
          v-for="run in [...filteredHistory].reverse()"
          :key="run.id"
          @click="emit('select-run', run)"
          class="timeline-entry group"
        >
          <!-- Timeline Marker -->
          <div
            class="entry-marker"
            :class="run.error ? 'entry-marker--error' : 'entry-marker--success'"
          ></div>

          <div class="entry-card">
            <div class="entry-header">
              <div class="entry-info">
                <div class="entry-label-row">
                  <span class="entry-name">{{ run.automation_name }}</span>
                  <span v-if="!selectedWorkspace" class="entry-workspace-tag">
                    {{ run.workspace_id || "Global" }}
                  </span>
                </div>
                <span class="entry-timestamp">{{
                  new Date(run.timestamp).toLocaleString()
                }}</span>
              </div>
              <div class="entry-meta">
                <span class="entry-model">{{
                  run.model || "System Default"
                }}</span>
                <span class="entry-duration"
                  >{{ run.duration_ms }}ms execution</span
                >
              </div>
            </div>

            <!-- Preview Area -->
            <div class="entry-preview-area">
              <div v-if="run.error" class="error-preview">
                <span class="error-badge">[Fail]</span> {{ run.error }}
              </div>

              <div v-if="run.output" class="output-preview">
                <div class="output-text">
                  {{
                    run.output
                      .replace(/\[\d{4}-\d{2}-\d{2}.*?\].*?\n/g, "")
                      .replace(/\*\*Output:\*\*\n\n```(text)?\n/, "")
                      .replace(/\n```$/, "")
                  }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="ledger-footer">
        <span class="footer-text">End of operational ledger</span>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.dashboard-shell {
  @apply flex-1 flex flex-col p-8 overflow-y-auto bg-gray-900/40 relative;
}

.dashboard-inner {
  @apply max-w-4xl mx-auto w-full;
}

.header-section {
  @apply flex flex-col gap-4 mb-8 sm:mb-10;
}

.header-row {
  @apply flex items-start justify-between gap-3;
}

.header-left {
  @apply min-w-0 flex-1;
}

.main-title {
  @apply text-2xl sm:text-3xl font-black text-white tracking-tighter;
}

.dashboard-subtitle {
  @apply text-sm text-gray-400 mt-1;
}

.workspace-inline {
  @apply inline-flex items-center gap-1 ml-1;
}

.workspace-icon {
  @apply text-blue-400 text-sm shrink-0;
}

.workspace-name {
  @apply font-mono text-blue-300 truncate max-w-[200px] sm:max-w-xs align-middle;
}

.workspace-clear {
  @apply ml-1 text-gray-500 hover:text-red-400 transition-colors text-xs shrink-0 align-middle;
}

.filter-bar {
  @apply flex items-center gap-1 pb-3 border-b border-white/5;
}

.btn-filter {
  @apply px-2 py-1 text-sm text-gray-500 hover:text-gray-200 transition-colors;
}

.btn-filter--active {
  @apply text-white;
}

.spacer {
  @apply flex-1;
}

.loader-spinner {
  @apply animate-spin h-4 w-4 border-2 border-blue-500 border-t-transparent rounded-full;
}

.empty-state {
  @apply flex flex-col items-center justify-center h-64 border-2 border-dashed border-gray-700/50 rounded-2xl;
}

.empty-state-text {
  @apply text-gray-500 italic text-sm;
}

.timeline-stream {
  @apply relative pl-8 space-y-12;
}

.timeline-rail {
  @apply absolute left-3 top-2 bottom-2 w-[2px] bg-gradient-to-b from-blue-500/50 via-gray-700 to-gray-800/0;
}

.timeline-entry {
  @apply relative cursor-pointer;
}

.entry-marker {
  @apply absolute -left-[25px] top-1.5 w-4 h-4 rounded-full border-2 border-gray-900 z-10 
         transition-transform group-hover:scale-125;
}

.entry-marker--error {
  @apply bg-red-500 shadow-[0_0_10px_rgba(239,68,68,0.5)];
}

.entry-marker--success {
  @apply bg-green-500 shadow-[0_0_10px_rgba(34,197,94,0.5)];
}

.entry-card {
  @apply bg-gray-800/40 border border-white/5 rounded-2xl p-6 hover:bg-gray-800/80 
         hover:border-gray-500/20 transition-all duration-300 shadow-xl group-hover:translate-x-1;
}

.entry-header {
  @apply flex flex-col sm:flex-row sm:items-start justify-between gap-3 mb-4 sm:mb-6;
}

.entry-info {
  @apply flex flex-col gap-1;
}

.entry-label-row {
  @apply flex items-center gap-3;
}

.entry-name {
  @apply text-xs font-black text-white tracking-tight uppercase;
}

.entry-workspace-tag {
  @apply px-2 py-0.5 bg-gray-700/50 text-[9px] text-gray-400 font-bold rounded uppercase tracking-widest border border-white/5;
}

.entry-timestamp {
  @apply text-[10px] text-gray-500 font-mono;
}

.entry-meta {
  @apply flex flex-row sm:flex-col items-center sm:items-end gap-2 sm:gap-1 text-gray-500 overflow-hidden;
}

.entry-model {
  @apply text-[10px] font-bold text-gray-400 tracking-widest uppercase truncate max-w-full;
}

.entry-duration {
  @apply text-[10px] text-gray-600 font-mono;
}

.entry-preview-area {
  @apply relative;
}

.error-preview {
  @apply bg-red-900/10 border border-red-900/20 rounded-lg p-4 text-[11px] text-red-300 font-mono;
}

.error-badge {
  @apply text-red-500 font-bold uppercase mr-2;
}

.output-preview {
  @apply bg-black/10 border border-white/5 rounded-lg p-4 text-[11px] text-gray-400 font-mono relative 
         overflow-x-auto group-hover:bg-black/30 transition-colors;
}

.output-text {
  @apply line-clamp-4 leading-[1.1] whitespace-pre;
}

.ledger-footer {
  @apply mt-16 text-center;
}

.footer-text {
  @apply text-[10px] font-bold text-gray-600 uppercase tracking-[0.3em];
}
</style>
