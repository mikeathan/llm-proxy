<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from "vue";
import type { AgentEvent } from "../../../types/dispatcher";
import {
  getRoleLabel,
  getMessageClass,
  getRoleClass,
  getContentClass,
} from "../../../domain/assistant";
import {
  getStepPayload,
  getMsgPayload,
  getToolCallPayload,
  getToolResPayload,
  getViolationPayload,
  formatEventsToText,
} from "../../../utils/dispatcher";
import { marked } from "marked";
import CopyButton from "../../common/CopyButton.vue";

const props = defineProps<{
  workspaceId: string;
  isActive: boolean;
  historyEvents?: AgentEvent[];
}>();

const liveEvents = ref<AgentEvent[]>([]);
const isConnected = ref(false);
let eventSource: EventSource | null = null;

const displayEvents = computed(() => {
  return liveEvents.value.length > 0
    ? liveEvents.value
    : props.historyEvents || [];
});

const connect = () => {
  if (eventSource) eventSource.close();

  const url = `/admin/api/dispatcher/workspaces/${props.workspaceId}/live`;
  eventSource = new EventSource(url);

  eventSource.addEventListener("ping", () => {
    isConnected.value = true;
  });

  eventSource.addEventListener("agent_update", (e) => {
    try {
      const ev = JSON.parse(e.data) as AgentEvent;
      liveEvents.value.push(ev);
    } catch (err) {
      console.error("Failed to parse agent event", err);
    }
  });

  eventSource.onerror = () => {
    isConnected.value = false;
  };
};

onMounted(() => {
  connect();
});

onUnmounted(() => {
  if (eventSource) eventSource.close();
});

watch(
  () => props.workspaceId,
  () => {
    liveEvents.value = [];
    connect();
  },
);

const fullTerminalText = computed(() => formatEventsToText(displayEvents.value));
</script>

<template>
  <div class="terminal-shell">
    <div class="terminal-header">
      <div class="status-indicator">
        <span class="dot" :class="{ 'dot--active': isConnected }"></span>
        <span class="status-text">
          <template v-if="liveEvents.length > 0">Live Stream</template>
          <template v-else-if="historyEvents?.length">Audit Log</template>
          <template v-else>Terminal</template>
        </span>
      </div>
      <div class="flex items-center gap-4">
        <CopyButton
          v-if="displayEvents.length > 0"
          :text="fullTerminalText"
          iconSize="sm"
          class="btn-clear-term"
        />
      </div>
    </div>

    <div class="terminal-body" id="terminal-scroll-area">
      <div v-if="displayEvents.length === 0" class="term-empty">
        Waiting for activity in {{ workspaceId }}...
      </div>

      <div class="term-content">
        <div v-for="(ev, i) in displayEvents" :key="i" class="term-line">
          <!-- Step Block -->
          <div v-if="ev.type === 'step_start'" class="line-step">
            <span class="step-label">Step {{ getStepPayload(ev).step }}</span>
          </div>

          <!-- Message Block -->
          <div
            v-else-if="(ev.type === 'message' || ev.type === 'error') && getMsgPayload(ev).content"
            class="line-msg"
            :class="getMessageClass(getMsgPayload(ev).role, ev.type)"
          >
            <span
              class="msg-role"
              :class="getRoleClass(getMsgPayload(ev).role)"
            >
              {{ getRoleLabel(getMsgPayload(ev).role) }}
            </span>
            <div
              v-if="getMsgPayload(ev).role === 'assistant'"
              class="msg-content prose prose-invert max-w-none text-[11px] leading-snug"
              v-html="marked.parse(getMsgPayload(ev).content)"
            ></div>
            <span
              v-else
              class="msg-content whitespace-pre-wrap"
              :class="getContentClass(getMsgPayload(ev).role)"
            >
              {{ getMsgPayload(ev).content }}
            </span>
          </div>

          <!-- Tool Call -->
          <div v-else-if="ev.type === 'tool_call'" class="line-tool">
            <div class="tool-run-header">
              <span class="tool-icon">🛠️</span>
              <span class="tool-name"
                >Executing {{ getToolCallPayload(ev).function.name }}...</span
              >
              <CopyButton
                :text="getToolCallPayload(ev).function.arguments"
                iconSize="sm"
                class="btn-copy-mini"
              />
            </div>
            <pre class="tool-args">{{
              getToolCallPayload(ev).function.arguments
            }}</pre>
          </div>

          <!-- Tool Result -->
          <div v-else-if="ev.type === 'tool_result'" class="line-result">
            <details class="res-details">
              <summary class="res-summary">
                <span class="res-icon">✅</span>
                <span class="res-name"
                  >{{ getToolResPayload(ev).name }} finished</span
                >
                <CopyButton
                  :text="getToolResPayload(ev).result"
                  class="btn-copy-mini ml-1"
                />
                <span class="res-hint ml-1">(click to view)</span>
              </summary>
              <pre class="res-body">{{
                typeof getToolResPayload(ev).result === "string"
                  ? getToolResPayload(ev).result
                  : JSON.stringify(getToolResPayload(ev).result, null, 2)
              }}</pre>
            </details>
          </div>

          <!-- Guardrail Violation -->
          <div v-else-if="ev.type === 'guardrail_violation'" class="line-violation">
            <div class="violation-header">
              <span class="violation-icon">🛑</span>
              <span class="violation-title">Guardrail Blocked: {{ getViolationPayload(ev).tool }}</span>
            </div>
            <div class="violation-body">{{ getViolationPayload(ev).error }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.terminal-shell {
  @apply bg-gray-900 border border-gray-800 rounded-lg flex flex-col h-[520px] shadow-2xl overflow-hidden font-mono text-[12px] text-gray-300;
}

.terminal-header {
  @apply px-4 py-2 border-b border-gray-800 flex justify-between items-center bg-gray-800/20 select-none;
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

.btn-clear-term {
  @apply text-[10px] text-gray-500 hover:text-white uppercase font-bold tracking-widest transition-colors;
}

.terminal-body {
  @apply flex-1 overflow-y-auto p-4 bg-[#0d1117] leading-relaxed;
}

.term-empty {
  @apply h-full flex items-center justify-center text-gray-600 italic text-[11px];
}

.term-content {
  @apply space-y-3;
}

.term-line {
  @apply max-w-full overflow-hidden;
}

/* Step Label */
.line-step {
  @apply mt-4 mb-2;
}

.step-label {
  @apply text-blue-400 font-bold uppercase;
}

/* Message */
.line-msg {
  @apply mb-2;
}

.msg-role {
  @apply mr-2;
}

.role-system {
  @apply text-indigo-400 font-bold uppercase tracking-wider text-[10px];
}

.role-assistant {
  @apply text-emerald-400 font-bold;
}

.role-user {
  @apply text-amber-400 font-bold;
}

.msg-content {
  @apply text-gray-300 flex-1;
}

/* Compact Typography Overrides for Terminal */
.msg-content :deep(p) {
  @apply mb-1 mt-0;
}
.msg-content :deep(h1), 
.msg-content :deep(h2), 
.msg-content :deep(h3) {
  @apply text-gray-100 font-bold mt-2 mb-1;
}
.msg-content :deep(h3) { @apply text-[11px]; }
.msg-content :deep(h2) { @apply text-[12px]; }
.msg-content :deep(h1) { @apply text-[14px]; }

.msg-content :deep(ul), .msg-content :deep(ol) {
  @apply mb-1 ml-4;
}

.msg-content :deep(table) {
  @apply my-2 border-collapse text-[10px] w-full;
}

.msg-content :deep(th), .msg-content :deep(td) {
  @apply p-1 border border-gray-800 text-left;
}

.system-msg {
  @apply border-l-2 border-indigo-500/30 pl-2 bg-indigo-500/5 py-1 my-1;
}

.system-error-msg {
  @apply border-l-2 border-red-500/30 pl-2 bg-red-500/10 py-1 my-1;
}

.content-system {
  @apply text-indigo-200/60 italic;
}

/* Tool execution block */
.line-tool {
  @apply mt-2 mb-2;
}

.tool-run-header {
  @apply flex items-center gap-2 mb-1.5;
}

.tool-icon {
  @apply text-sm opacity-90;
}

.tool-name {
  @apply text-blue-300 font-semibold;
}

.btn-copy-mini {
  @apply p-1 text-gray-500 hover:text-gray-300 transition-colors ml-1 focus:outline-none;
}

.tool-args {
  @apply bg-[#161b22] border border-gray-800 rounded p-3 text-[11px] text-blue-200/70 overflow-x-auto whitespace-pre-wrap mt-1;
}

/* Tool Result Block */
.line-result {
  @apply mt-2 mb-4;
}

.res-details {
  @apply cursor-pointer outline-none;
}

.res-summary {
  @apply flex items-center gap-2 select-none hover:opacity-80 transition-opacity outline-none list-none;
}

.res-summary::-webkit-details-marker {
  display: none;
}

.res-icon {
  @apply text-sm;
}

.res-name {
  @apply text-green-400 font-semibold;
}

.res-hint {
  @apply text-gray-600 text-[10px] italic;
}

.res-body {
  @apply bg-[#161b22] border border-gray-800 rounded p-3 mt-2 text-[11px] text-green-500/80 overflow-y-auto max-h-80 whitespace-pre-wrap;
}

.line-violation {
  @apply mb-4 p-3 bg-red-900/20 border border-red-500/30 rounded-lg animate-in fade-in slide-in-from-left-2 backdrop-blur-sm;
}

.violation-header {
  @apply flex items-center gap-2 mb-1.5;
}

.violation-title {
  @apply text-red-400 font-bold text-xs uppercase tracking-widest;
}

.violation-body {
  @apply text-[11px] text-red-100/70 pl-6 border-l border-red-500/30 py-1;
}
</style>
