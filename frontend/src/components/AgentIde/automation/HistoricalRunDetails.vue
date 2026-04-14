<script setup lang="ts">
import type { AutomationRun } from "../../../types/dispatcher";
import MarkdownViewer from "../../common/MarkdownViewer.vue";
import ExecutionAuditTrail from "./ExecutionAuditTrail.vue";
import { formatTime } from "../../../utils/time";

const props = defineProps<{
  run: AutomationRun;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();



</script>

<template>
  <div class="details-shell">
    <div class="header-section">
      <div class="title-row">
        <h2 class="main-title">
          <span
            class="status-dot"
            :class="run.error ? 'status-dot--error' : 'status-dot--success'"
          ></span>
          <span class="title-prefix">History /</span> {{ run.automation_name }}
        </h2>
        <div class="header-actions">
          <span class="run-id-tag">{{ run.id }}</span>
          <button
            @click="emit('close')"
            class="btn-close group"
            title="Close and return to dashboard"
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
      </div>

      <div class="stats-grid">
        <div class="stat-box">
          <span class="stat-label">Status</span>
          <span
            class="stat-value"
            :class="run.error ? 'stat-value--error' : 'stat-value--success'"
          >
            {{ run.error ? "Failed" : "Success" }}
          </span>
        </div>
        <div class="stat-box">
          <span class="stat-label">Duration</span>
          <span class="stat-value stat-value--mono"
            >{{ run.duration_ms }} ms</span
          >
        </div>
        <div class="stat-box">
          <span class="stat-label">Model</span>
          <span class="stat-value">{{ run.model || "Default" }}</span>
        </div>
      </div>
    </div>

    <div class="content-section">
      <!-- Full Terminal Execution Log (Replay) -->
      <ExecutionAuditTrail v-if="run.events?.length" :events="run.events" />


      <div v-if="run.error" class="error-section">
        <h4 class="section-header section-header--error">Final Error</h4>
        <div class="error-box">
          {{ run.error }}
        </div>
      </div>

      <div v-if="run.output" class="output-section">
        <h4 class="section-header section-header--info">Final Summary Report</h4>
        <div class="output-box">
          <MarkdownViewer :content="run.output" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.details-shell {
  @apply flex-1 flex flex-col h-full animate-in fade-in zoom-in-95 duration-300;
}

.header-section {
  @apply p-6 border-b border-gray-700 bg-gray-900/10;
}

.title-row {
  @apply flex items-center justify-between mb-4;
}

.main-title {
  @apply text-xl font-bold text-gray-100 flex items-center gap-3;
}

.status-dot {
  @apply w-3 h-3 rounded-full;
}

.status-dot--error {
  @apply bg-red-500;
}

.status-dot--success {
  @apply bg-green-500;
}

.title-prefix {
  @apply text-gray-500 text-sm font-normal;
}

.header-actions {
  @apply flex items-center gap-4;
}

.run-id-tag {
  @apply text-[10px] px-2 py-1 bg-gray-700/50 rounded text-gray-400 font-mono border border-white/5;
}

.btn-close {
  @apply bg-gray-700 hover:bg-gray-600 text-white p-1.5 rounded-full transition-colors 
         flex items-center justify-center shadow-lg active:scale-95;
}

.stats-grid {
  @apply grid grid-cols-3 gap-6;
}

.stat-box {
  @apply bg-gray-800/40 p-3 rounded-lg border border-white/5;
}

.stat-label {
  @apply text-[10px] uppercase font-bold text-gray-500 block mb-1;
}

.stat-value {
  @apply text-sm font-medium text-gray-300;
}

.stat-value--error {
  @apply text-red-400;
}

.stat-value--success {
  @apply text-green-400;
}

.stat-value--mono {
  @apply font-mono;
}

.content-section {
  @apply flex-1 p-6 overflow-y-auto bg-gray-900/20 space-y-8;
}

.section-header--info {
  @apply text-blue-500/80;
}

.error-box {

  @apply bg-red-900/10 border border-red-900/20 p-4 rounded-lg font-mono text-sm text-red-300 whitespace-pre-wrap;
}

.output-box {
  @apply bg-gray-950/40 border border-white/5 p-6 rounded-xl shadow-2xl;
}
</style>
