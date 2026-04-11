<script setup lang="ts">
import { ref } from "vue";
import type { Automation } from "../../../types/dispatcher";
import MarkdownViewer from "../../common/MarkdownViewer.vue";

defineProps<{
  automation: Automation;
  lastTriggerResult?: string | null;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const showHistory = ref(false);
</script>

<template>
  <div class="details-shell">
    <div class="details-header">
      <div class="details-title-inner">
        <h2 class="details-title">
          <span class="title-path">automation /</span> {{ automation.name }}
        </h2>
        <p class="details-subtitle">
          Workspace Scope: <span class="details-subtitle-text">{{ automation.workspace }}</span>
        </p>
      </div>
      <button 
        @click="emit('close')"
        class="btn-close-round group"
        title="Close details and return to dashboard"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>

    <div class="details-content">
      <div class="meta-grid">
        <div class="meta-card">
          <span class="meta-label">Trigger</span>
          <span class="meta-value meta-value--primary">{{ automation.trigger }}</span>
        </div>
        <div class="meta-card">
          <span class="meta-label">Strategy</span>
          <span class="meta-value meta-value--secondary">{{ automation.strategy }}</span>
        </div>
        <div class="meta-card">
          <span class="meta-label">Task File</span>
          <span class="meta-value meta-value--mono">{{ automation.task_file }}</span>
        </div>
      </div>

      <div
        v-if="lastTriggerResult"
        class="result-banner"
        :class="lastTriggerResult.includes('Failed') ? 'result-banner--error' : 'result-banner--success'"
      >
        {{ lastTriggerResult }}
      </div>

      <div v-if="automation.last_error" class="error-section">
        <h4 class="section-title section-title--error">Last Error</h4>
        <div class="error-box">
          {{ automation.last_error }}
        </div>
      </div>

      <div v-if="automation.last_output" class="output-section">
        <div class="output-header">
          <h4 class="section-title section-title--success">Last Execution Output</h4>
          <button 
            v-if="automation.history && automation.history.length > 0"
            @click="showHistory = !showHistory"
            class="btn-history-toggle"
          >
            {{ showHistory ? 'Show Current' : 'View History' }}
          </button>
        </div>
        
        <div v-if="!showHistory" class="output-box">
          <MarkdownViewer :content="automation.last_output" />
        </div>

        <!-- History List -->
        <div v-else class="history-list">
          <div 
            v-for="run in [...(automation.history || [])].reverse()" 
            :key="run.id"
            class="history-item"
          >
            <div class="history-meta">
              <div class="history-meta-content">
                <span class="history-timestamp">{{ new Date(run.timestamp).toLocaleString() }}</span>
                <div class="history-status-row">
                  <span class="badge-status" :class="run.error ? 'badge-status--error' : 'badge-status--success'">
                    {{ run.error ? 'Error' : 'Success' }}
                  </span>
                  <span class="history-model">{{ run.model || 'Default Model' }}</span>
                </div>
              </div>
              <span class="history-duration">{{ run.duration_ms }}ms</span>
            </div>
            
            <div v-if="run.error" class="history-error-box">
              {{ run.error }}
            </div>
            
            <div v-if="run.output" class="history-output-box">
              <MarkdownViewer :content="run.output" />
            </div>
          </div>
        </div>
      </div>

      <div v-if="!automation.last_output && !automation.last_error" class="empty-state">
        <p>No execution history available for this automation.</p>
        <p class="empty-state-text">Run the automation to see the output here.</p>
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
  @apply bg-gray-900/50 border border-gray-700/50 rounded-lg p-5 shadow-inner animate-in fade-in duration-300;
}

.history-list {
  @apply space-y-3 animate-in slide-in-from-right duration-300;
}

.history-item {
  @apply bg-gray-900/40 border border-gray-700/30 rounded p-4 hover:border-gray-500/50 transition-colors;
}

.history-meta {
  @apply flex items-start justify-between mb-3;
}

.history-meta-content {
  @apply flex flex-col gap-1;
}

.history-timestamp {
  @apply text-[10px] font-mono text-gray-500;
}

.history-status-row {
  @apply flex gap-2 items-center;
}

.badge-status {
  @apply text-[9px] px-1.5 py-0.5 rounded font-bold uppercase;
}

.badge-status--success {
  @apply bg-green-900/40 text-green-400;
}

.badge-status--error {
  @apply bg-red-900/40 text-red-400;
}

.history-model {
  @apply text-xs text-gray-300 font-medium;
}

.history-duration {
  @apply text-[10px] text-gray-500 font-mono;
}

.history-error-box {
  @apply text-xs text-red-300 font-mono bg-red-900/10 p-2 rounded mb-2 border border-red-900/20;
}

.history-output-box {
  @apply bg-black/20 rounded p-3 border border-white/5 shadow-inner;
}

.empty-state {
  @apply text-gray-500 text-sm italic;
}

.empty-state-text {
  @apply text-xs mt-1;
}
</style>
