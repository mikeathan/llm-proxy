<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import type { Automation, AutomationRun } from "../../../types/dispatcher";
import MarkdownViewer from "../../common/display/MarkdownViewer.vue";
import ExecutionAuditTrail from "./ExecutionAuditTrail.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import Icon from "../../icons/Icon.vue";
import InlineConfirm from "../../ui/InlineConfirm.vue";
import ChatMessages from "../assistant/ChatMessages.vue";
import { useLiveConsole } from "../../../composables/automation/useLiveConsole";
import { groupTurns } from "../../../utils/message/turnGrouper";
import GuardrailBanner from "../../common/chat/GuardrailBanner.vue";
import { useTurnInset } from "../../../composables/ui/useTurnInset";

const props = defineProps<{
  automation: Automation;
  lastTriggerResult?: string | null;
  isExecuting?: boolean;
  selectedRun?: AutomationRun | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "delete-run", run: AutomationRun): void;
  (e: "delete-automation-runs", automation: Automation): void;
}>();

const confirmClearRuns = ref(false);
const confirmDeleteRunId = ref<string | null>(null);

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

// Final consolidated running state
const showLiveUI = computed(() => !!(props.isExecuting || props.automation?.is_running));

// Unified renderer: reuse the assistant ChatMessages view for automation runs.
// useLiveConsole streams the SSE channel and feeds AgentEvents through the
// shared useMessageBuilder (same single consumer as chat), so reasoning/tool-
// calls render as proper segments instead of being overwritten into a single
// assistant message.
const {
  messages: displayMessages,
  thinking,
  liveReasoning,
  paused,
  phase,
  isConnected,
  pendingDecision,
  connect,
  disconnect,
  clearEvents,
  submitDecision,
} = useLiveConsole(
  () => props.automation.workspace,
  () => showLiveUI.value,
  () => activeRun.value?.events,
  props.automation.name,
);

const automationTurns = computed(() => groupTurns(displayMessages.value));

const expandedSegments = ref<Record<string, boolean>>({});
const { insetCollapsed, isInsetCollapsed, toggleInset } = useTurnInset(phase, automationTurns);

function isSegExpanded(turnIdx: number, segIdx: number): boolean {
  return !!expandedSegments.value[`${turnIdx}-${segIdx}`];
}
function toggleSegment(turnIdx: number, segIdx: number) {
  const key = `${turnIdx}-${segIdx}`;
  expandedSegments.value = { ...expandedSegments.value, [key]: !expandedSegments.value[key] };
}

onMounted(() => {
  connect();
});

onUnmounted(() => {
  disconnect();
});

watch(
  () => props.automation.workspace,
  () => {
    clearEvents();
    connect();
  },
);
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
      <BaseButton
        variant="ghost"
        size="md"
        icon="close"
        iconOnly
        @click="emit('close')"
        className="!bg-gray-800 hover:!bg-gray-700 !text-gray-400 hover:!text-white"
        title="Close details and return to dashboard"
      />
    </div>

    <div class="details-content">
      <!-- STATIC INFO & PREVIOUS RESULTS (Only shown when NOT running) -->
      <div v-if="!showLiveUI" class="animate-in fade-in duration-500">
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
            <span class="meta-label">Model</span>
            <span class="meta-value meta-value--mono">{{
              automation.model || "Default"
            }}</span>
          </div>
          <div class="meta-card">
            <span class="meta-label">Task File</span>
            <span class="meta-value meta-value--mono">{{
              automation.task_file
            }}</span>
          </div>
          <div v-if="automation.recording_ref" class="meta-card meta-card--recording">
            <span class="meta-label">Recording</span>
            <span class="meta-value meta-value--recording">
              <span class="rec-dot"></span>
              {{ automation.recording_ref }}
            </span>
          </div>
        </div>

        <div v-if="automation.last_error" class="error-section">
          <h4 class="section-title section-title--error">Last Error</h4>
          <div class="error-box">
            {{ automation.last_error }}
          </div>
        </div>

        <div v-if="automation.last_output" class="output-section">
          <div class="output-header">
            <h4 class="section-title section-title--success">
              Latest Summary Report
            </h4>
            <div class="header-actions-right">
              <button
                v-if="automation.history && automation.history.length > 0"
                @click="showHistory = !showHistory"
                class="btn-history-toggle"
              >
                {{ showHistory ? "Back to Latest" : "Full Timeline History" }}
              </button>
              <button
                v-if="automation.history && automation.history.length > 0"
                @click="confirmClearRuns = !confirmClearRuns"
                class="btn-clear-runs"
                title="Delete every run directory for this automation"
              >
                Clear All Runs
              </button>
            </div>
            <InlineConfirm
              v-if="confirmClearRuns"
              message="Delete ALL run artifacts for this automation? This removes every run directory and cannot be undone."
              @confirm="emit('delete-automation-runs', automation); confirmClearRuns = false"
              @cancel="confirmClearRuns = false"
              class="clear-runs-confirm"
            />
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
              :class="{
                'history-entry--expanded': expandedHistoryRuns[run.id],
              }"
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
                  <button
                    class="entry-del-btn"
                    :title="`Delete this run`"
                    @click.stop="confirmDeleteRunId = run.id"
                  >
                    <Icon name="trash" size="xs" />
                  </button>
                  <Icon
                    v-if="!expandedHistoryRuns[run.id]"
                    name="close"
                    size="xs"
                    class="rotate-45"
                  />
                  <Icon v-else name="close" size="xs" class="text-blue-400" />
                </div>
              </div>

              <div v-if="confirmDeleteRunId === run.id" class="entry-confirm" @click.stop>
                <InlineConfirm
                  :message="`Delete this run? This removes the run and its artifacts and cannot be undone.`"
                  @confirm="confirmDeleteRunId = null; emit('delete-run', run)"
                  @cancel="confirmDeleteRunId = null"
                />
              </div>

              <div v-if="expandedHistoryRuns[run.id]" class="entry-details">
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
      </div>

      <!-- LIVE EXECUTION STATUS (Shown during trigger and active runs) -->
      <div v-if="showLiveUI" class="result-banner result-banner--success">
        <div class="flex items-center gap-3">
          <Icon name="spinner" size="xs" />
          <span class="font-bold tracking-tight uppercase text-[10px]">
            {{ lastTriggerResult || "Automation in progress..." }}
          </span>
        </div>
      </div>

      <!-- UNIFIED RENDERER (Always visible) -->
      <div class="console-section" :class="{ 'mt-12': !showLiveUI }">
        <div class="run-header">
          <h4 class="section-title section-title--accent">
            Operational Run
          </h4>
          <span class="status-indicator">
            <span class="dot" :class="{ 'dot--active': isConnected }"></span>
            <span class="status-text">
              <template v-if="displayMessages.length > 0">Live Stream</template>
              <template v-else-if="activeRun?.events?.length">Audit Log</template>
              <template v-else>Idle</template>
            </span>
          </span>
        </div>

        <GuardrailBanner
          v-if="pendingDecision"
          :decision="pendingDecision"
          @allow="(persist: boolean) => submitDecision(true, persist)"
          @deny="() => submitDecision(false, false)"
        />

        <div class="automation-run-shell">
          <ChatMessages
            mode="automation"
            :messages="displayMessages"
            :turns="automationTurns"
            :loading="showLiveUI"
            :thinking="thinking"
            :live-reasoning="liveReasoning"
            :paused="paused"
            :workspace-id="automation.workspace"
            :turns-collapsed="insetCollapsed"
            :expanded-segments="expandedSegments"
            :is-inset-collapsed="isInsetCollapsed"
            :is-seg-expanded="isSegExpanded"
            :phase="phase"
            @toggle-inset="toggleInset"
            @toggle-segment="toggleSegment"
          />
        </div>
      </div>

      <div
        v-if="!showLiveUI && !automation.last_output && !automation.last_error"
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

.meta-card--recording {
  @apply border-orange-700/40 bg-orange-900/10;
}

.meta-value--recording {
  @apply text-orange-400 text-xs font-mono flex items-center gap-1.5 truncate max-w-full;
}

.rec-dot {
  @apply w-1.5 h-1.5 rounded-full bg-orange-500 shrink-0;
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

.header-actions-right {
  @apply flex items-center gap-3;
}

.btn-clear-runs {
  @apply text-[10px] text-red-500/70 hover:text-red-400 uppercase tracking-widest font-bold transition-colors border border-red-500/30 rounded px-2 py-1;
}

.clear-runs-confirm {
  @apply mt-3;
}

.entry-del-btn {
  @apply text-gray-500 hover:text-red-400 transition-colors shrink-0 flex items-center justify-center;
}

.entry-confirm {
  @apply mt-1;
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

/* Unified automation run renderer — reuses the assistant chat layout */
.automation-run-shell {
  @apply bg-gray-900/40 border border-gray-800 rounded-lg overflow-hidden h-[520px] shadow-2xl;
}

.run-header {
  @apply flex items-center justify-between mb-3;
}

.status-indicator {
  @apply flex items-center gap-2;
}

.dot {
  @apply w-1.5 h-1.5 rounded-full bg-gray-600;
}

.dot--active {
  @apply bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)] animate-pulse;
}

.status-text {
  @apply text-[10px] font-bold uppercase tracking-widest text-gray-500;
}
</style>
