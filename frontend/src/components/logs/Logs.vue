<script setup lang="ts">
import { ref, nextTick, watch, onMounted } from "vue";
import LogLevelPanel from "../settings/LogLevelPanel.vue";
import { useLogs } from "../../composables/system/useLogs";
import { useMetrics } from "../../composables/system/useMetrics";
import { useAutoScroll } from "../../composables/ui/useAutoScroll";

type LogTab = "app" | "process";
const activeTab = ref<LogTab>("app");

const {
  processLogLines,
  processLogRunning,
  processLogName,
  processLogReady,
  appLogLines,
  appLogsFetched,
  appLogsActive,
  clearProcessLogs,
  clearAppLogs,
} = useLogs();

const { logLevel, updateLogLevel } = useMetrics();

// Auto-scroll refs
const processLogEl = ref<HTMLElement | null>(null);
const appLogEl = ref<HTMLElement | null>(null);

const scroller = useAutoScroll();

// Toggle lazy app log loading based on tab
watch(activeTab, (val) => {
  appLogsActive.value = (val === "app");
}, { immediate: true });

const isActive = (tab: LogTab) => activeTab.value === tab;

// Watch for content or tab changes to trigger scroll
watch([appLogLines, processLogLines, activeTab], async () => {
  const el = isActive("app") ? appLogEl.value : processLogEl.value;
  scroller.notifyContent();
  await nextTick();
  scroller.scrollIfNearBottom(el, "auto");
});

onMounted(() => {
  const el = isActive("app") ? appLogEl.value : processLogEl.value;
  if (el) el.scrollTop = el.scrollHeight;
});

const handleClear = () => {
  if (isActive("app")) clearAppLogs();
  else clearProcessLogs();
};
</script>

<template>
  <div class="logs-shell">
    <!-- ── Header ── -->
    <header class="logs-header">
      <nav class="logs-tabs">
        <button
          id="tab-app-logs"
          class="tab-btn"
          :class="isActive('app') ? 'tab-btn--active' : 'tab-btn--inactive'"
          @click="activeTab = 'app'"
        >
          <span class="tab-icon">📋</span>
          App Logs
        </button>
        <button
          id="tab-process-logs"
          class="tab-btn"
          :class="isActive('process') ? 'tab-btn--active' : 'tab-btn--inactive'"
          @click="activeTab = 'process'"
        >
          <span class="tab-icon">⚙️</span>
          Process Logs
          <span v-if="processLogRunning" class="running-dot">
            <span class="running-ping"></span>
            <span class="running-inner"></span>
          </span>
        </button>
      </nav>

      <div class="logs-controls">
        <LogLevelPanel :modelValue="logLevel" @update="updateLogLevel" />

        <div v-if="isActive('process')" class="process-status">
          <span v-if="processLogRunning" class="badge-running">
            {{ processLogName }}<span v-if="processLogReady"> · ready</span>
          </span>
          <span v-else class="badge-stopped">stopped</span>
        </div>

        <button id="btn-clear-logs" class="btn-clear" @click="handleClear">
          Clear
        </button>
      </div>
    </header>

    <!-- ── Content Panes ── -->
    <main class="logs-content">
      <div v-show="isActive('app')" class="logs-pane" ref="appLogEl" @scroll="scroller.updateWasNearBottom(appLogEl)">
        <pre class="log-text">{{ 
          appLogLines || (appLogsFetched ? "No application logs available." : "Loading application logs...") 
        }}</pre>
      </div>

      <div v-show="isActive('process')" class="logs-pane" ref="processLogEl" @scroll="scroller.updateWasNearBottom(processLogEl)">
        <pre class="log-text">{{ processLogLines || "No process logs available." }}</pre>
      </div>
    </main>
  </div>
</template>

<style scoped lang="postcss">
.logs-shell {
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 flex flex-col overflow-hidden;
  height: 76vh;
  min-height: 400px;
}

.logs-header {
  @apply flex items-center justify-between gap-3 px-4 py-2.5 border-b border-gray-700 bg-gray-800 flex-wrap;
}

.logs-tabs {
  @apply flex gap-1;
}

.tab-btn {
  @apply flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-all;
}

.tab-btn--active {
  @apply bg-gray-700 text-white shadow-inner;
}

.tab-btn--inactive {
  @apply text-gray-400 hover:text-gray-200 hover:bg-gray-700/50;
}

.tab-icon {
  @apply text-xs;
}

.running-dot {
  @apply relative flex h-2 w-2 ml-1;
}

.running-ping {
  @apply animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75;
}

.running-inner {
  @apply relative inline-flex rounded-full h-2 w-2 bg-green-500;
}

.logs-controls {
  @apply flex items-center gap-4 flex-wrap;
}

.process-status {
  @apply flex items-center;
}

.badge-running {
  @apply text-xs text-green-400 font-mono;
}

.badge-stopped {
  @apply text-xs text-yellow-500;
}

.btn-clear {
  @apply px-3 py-1 bg-gray-700 hover:bg-gray-600 text-gray-300 hover:text-white text-xs rounded transition-colors border border-gray-600;
}

.logs-content {
  @apply flex-1 relative overflow-hidden;
}

.logs-pane {
  @apply absolute inset-0 overflow-y-auto bg-[#1a1a1a] p-4;
}

.log-text {
  @apply font-mono text-xs text-gray-300 leading-relaxed whitespace-pre-wrap break-words select-text m-0;
}
</style>
