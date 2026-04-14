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
} from "../../../utils/dispatcher";
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

  const url = `http://localhost:4001/admin/api/dispatcher/workspaces/${props.workspaceId}/live`;
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

const clearLogs = () => {
  liveEvents.value = [];
};
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
      <button @click="clearLogs" class="btn-clear-term">Clear</button>
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
            v-else-if="ev.type === 'message' && getMsgPayload(ev).content"
            class="line-msg"
            :class="getMessageClass(getMsgPayload(ev).role)"
          >
            <span
              class="msg-role"
              :class="getRoleClass(getMsgPayload(ev).role)"
            >
              {{ getRoleLabel(getMsgPayload(ev).role) }}
            </span>
            <span
              class="msg-content"
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
  @apply text-purple-400 font-bold mr-2;
}

.msg-content {
  @apply text-gray-300 whitespace-pre-wrap;
}

.system-error-msg {
  @apply border-l-2 border-red-500/50 pl-2 bg-red-900/10 py-1;
}

.role-error {
  @apply text-red-500 font-black;
}

.content-error {
  @apply text-red-400 font-bold;
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
</style>
