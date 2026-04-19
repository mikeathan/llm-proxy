<script setup lang="ts">
import { ref, computed } from "vue";
import type { Automation, AutomationRun } from "../../../types/dispatcher";
import MarkdownViewer from "../../common/MarkdownViewer.vue";
import LiveConsole from "./LiveConsole.vue";
import ExecutionAuditTrail from "./ExecutionAuditTrail.vue";

const props = defineProps<{
  automation: Automation;
  lastTriggerResult?: string | null;
  selectedRun?: AutomationRun | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const showHistory = ref(false);
const expandedHistoryRuns = ref<Record<string, boolean>>({});

// Use either the explicitly selected run (from Pulse) or the very latest one
const activeRun = computed(() => {
  if (props.selectedRun) return props.selectedRun;
  if (!props.automation.history || props.automation.history.length === 0)
    return null;
  return [...props.automation.history].sort(
    (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
  )[0];
});

const toggleHistoryRun = (runId: string) => {
  expandedHistoryRuns.value[runId] = !expandedHistoryRuns.value[runId];
};

const isExecuting = computed(() => {
  return props.automation.is_running || props.lastTriggerResult?.toLowerCase().includes("running") || false;
});
</script>

<template>
  <div class="details-shell">
    <div class="details-header">
      <div class="details-title-inner">
        <h2 class="details-title">
          <span class="title-path">automation /</span> {{ automation.name }}
        </h2>
        <p class="details-subtitle">
          Workspace Scope:
          <span class="details-subtitle-text">{{ automation.workspace }}</span>
        </p>
      </div>
      <button
        @click="emit('close')"
        class="btn-close-round group"
        title="Close details and return to dashboard"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-4 w-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M6 18L18 6M6 6l12 12"
          />
        </svg>
      </button>
    </div>

    <div class="details-content">
      <template v-if="!isExecuting">
        <div class="meta-grid">
          <div class="meta-card">
            <span class="meta-label">Trigger</span>
            <span class="meta-value meta-value--primary">{{
              automation.trigger
            }}</span>
          </div>
          <div class="meta-card">
            <span class="meta-label">Strategy</span>
            <span class="meta-value meta-value--secondary">{{
              automation.strategy
            }}</span>
          </div>
          <div class="meta-card">
            <span class="meta-label">Task File</span>
            <span class="meta-value meta-value--mono">{{
              automation.task_file
            }}</span>
          </div>
        </div>
      </template>

      <div
        v-if="lastTriggerResult"
        class="result-banner"
        :class="
          lastTriggerResult.includes('Failed')
            ? 'result-banner--error'
            : 'result-banner--success'
        "
      >
        <div v-if="isExecuting" class="flex items-center gap-3">
          <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-current"></div>
          <span class="font-bold tracking-tight uppercase text-[10px]">{{ lastTriggerResult }}</span>
        </div>
        <span v-else>{{ lastTriggerResult }}</span>
      </div>

      <template v-if="!isExecuting">
        <div v-if="automation.last_error" class="error-section">
          <h4 class="section-title section-title--error">Last Error</h4>
          <div class="error-box">
            {{ automation.last_error }}
          </div>
        </div>
      </template>

      <!-- Execution Summary / Report Section -->
      <template v-if="!isExecuting">
        <div v-if="automation.last_output" class="output-section">
        <div class="output-header">
          <h4 class="section-title section-title--success">
            Latest Summary Report
          </h4>
          <button
            v-if="automation.history && automation.history.length > 0"
            @click="showHistory = !showHistory"
            class="btn-history-toggle"
          >
            {{ showHistory ? "Back to Latest" : "Full Timeline History" }}
          </button>
        </div>

        <div v-if="!showHistory" class="output-box">
          <MarkdownViewer :content="automation.last_output" />
        </div>

        <!-- History Timeline -->
        <div v-else class="history-timeline">
          <div
            v-for="run in [...(automation.history || [])].reverse()"
            :key="run.id"
            class="history-entry"
            :class="{ 'history-entry--expanded': expandedHistoryRuns[run.id] }"
          >
            <div @click="toggleHistoryRun(run.id)" class="entry-header">
              <div class="entry-meta">
                <span
                  class="entry-dot"
                  :class="run.error ? 'bg-red-500' : 'bg-green-500'"
                ></span>
                <span class="entry-time">{{
                  new Date(run.timestamp).toLocaleString()
                }}</span>

                <span class="entry-model"
                  >via {{ run.model || "Default" }}</span
                >
              </div>
              <div class="entry-actions">
                <span class="entry-duration">{{ run.duration_ms }}ms</span>
                <svg
                  v-if="!expandedHistoryRuns[run.id]"
                  xmlns="http://www.w3.org/2000/svg"
                  class="h-3 w-3 text-gray-500"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M19 9l-7 7-7-7"
                  />
                </svg>
                <svg
                  v-else
                  xmlns="http://www.w3.org/2000/svg"
                  class="h-3 w-3 text-blue-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 15l7-7 7 7"
                  />
                </svg>
              </div>
            </div>

            <!-- Expanded Audit Trail -->
            <div v-if="expandedHistoryRuns[run.id]" class="entry-details">
              <!-- Replay terminal events using the centralized ExecutionAuditTrail -->
              <ExecutionAuditTrail
                v-if="run.events?.length"
                :events="run.events"
              />


              <div v-if="run.error" class="entry-error">
                <h5 class="sub-header">Final Error</h5>
                <pre>{{ run.error }}</pre>
              </div>

              <div v-if="run.output" class="entry-output">
                <h5 class="sub-header">Final Report Output</h5>
                <MarkdownViewer :content="run.output" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

      <!-- HYBRID CONSOLE SECTION -->
      <div class="console-section" :class="{ 'mt-12': !isExecuting }">
        <h4 class="section-title section-title--accent">
          Operational Terminal
        </h4>

        <!-- Always mounted LiveConsole handles both live and history automatically -->
        <LiveConsole
          :workspaceId="automation.workspace"
          :isActive="true"
          :historyEvents="activeRun?.events"
        />
      </div>

      <div
        v-if="!automation.last_output && !automation.last_error"
        class="empty-state"
      >
        <p>No execution history available for this automation.</p>
        <p class="empty-state-text">
          Run the automation to see the output here.
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.details-shell {
  @apply flex-1 flex flex-col h-full animate-in fade-in zoom-in-95 duration-300;
}

.details-header {
  @apply p-6 border-b border-gray-700 bg-gray-900/10 flex items-center justify-between;
}

.details-title {
  @apply text-xl font-bold text-gray-100 flex items-center gap-3;
}

.title-path {
  @apply text-blue-500 text-sm font-normal italic;
}

.details-subtitle {
  @apply text-[10px] text-gray-500 font-bold uppercase tracking-widest mt-1;
}

.details-subtitle-text {
  @apply text-gray-400;
}

.btn-close-round {
  @apply bg-gray-700 hover:bg-gray-600 text-white p-1.5 rounded-full transition-colors flex items-center justify-center;
}

.details-content {
  @apply flex-1 p-8 overflow-y-auto bg-gray-900/10;
}

.meta-grid {
  @apply grid grid-cols-1 md:grid-cols-3 gap-6 mb-12;
}

.meta-card {
  @apply bg-gray-800/40 p-4 rounded-xl border border-white/5;
}

.meta-label {
  @apply text-[10px] uppercase font-bold text-gray-500 block mb-2 tracking-widest;
}

.meta-value {
  @apply text-sm font-medium;
}

.meta-value--primary {
  @apply text-blue-400 font-mono;
}

.meta-value--secondary {
  @apply text-gray-300;
}

.meta-value--mono {
  @apply text-gray-300 font-mono;
}

.result-banner {
  @apply p-4 rounded-xl text-sm mb-8 border animate-in zoom-in-95 duration-300;
}

.result-banner--success {
  @apply bg-green-900/10 border-green-900/30 text-green-300;
}

.result-banner--error {
  @apply bg-red-900/10 border-red-900/30 text-red-300;
}

.error-section {
  @apply mb-6;
}

.section-title {
  @apply text-xs font-bold uppercase tracking-wider mb-2;
}

.section-title--error {
  @apply text-red-400;
}

.section-title--success {
  @apply text-blue-400;
}

.section-title--accent {
  @apply text-purple-400;
}

.error-box {
  @apply bg-red-900/20 border border-red-900/50 rounded p-3 text-red-200 text-sm font-mono whitespace-pre-wrap;
}

.output-section {
  @apply mb-6;
}

.output-header {
  @apply flex items-center justify-between mb-3;
}

.btn-history-toggle {
  @apply text-[10px] text-gray-500 hover:text-blue-400 uppercase tracking-widest font-bold transition-colors;
}

.output-box {
  @apply bg-gray-900/50 border border-gray-700/50 rounded-lg p-6 shadow-inner animate-in fade-in duration-300;
}

/* History Timeline */
.history-timeline {
  @apply space-y-4 animate-in slide-in-from-right duration-300 pb-4;
}

.history-entry {
  @apply bg-gray-900/60 border border-white/5 rounded-lg overflow-hidden transition-all duration-200;
}

.history-entry:hover {
  @apply border-gray-600/50 bg-gray-800/40;
}

.history-entry--expanded {
  @apply border-blue-500/30 bg-gray-900/80 shadow-2xl shadow-blue-900/10;
}

.entry-header {
  @apply px-4 py-3 flex items-center justify-between cursor-pointer select-none;
}

.entry-meta {
  @apply flex items-center gap-3;
}

.entry-dot {
  @apply w-1.5 h-1.5 rounded-full;
}

.entry-time {
  @apply text-[11px] text-gray-300 font-bold;
}

.entry-model {
  @apply text-[10px] text-gray-600 font-mono;
}

.entry-actions {
  @apply flex items-center gap-4;
}

.entry-duration {
  @apply text-[10px] text-gray-500 font-mono;
}

.entry-details {
  @apply p-4 pt-0 border-t border-gray-800/50 bg-black/20 animate-in slide-in-from-top duration-200;
}




.sub-header {
  @apply text-[9px] uppercase font-bold text-gray-500 mb-2 tracking-widest;
}

.entry-error pre {
  @apply bg-red-900/10 p-3 rounded text-red-300 text-[10px] font-mono border border-red-900/20 whitespace-pre-wrap;
}

.entry-output {
  @apply mt-4;
}

.empty-state {
  @apply text-gray-500 text-sm italic;
}

.empty-state-text {
  @apply text-xs mt-1;
}
</style>
