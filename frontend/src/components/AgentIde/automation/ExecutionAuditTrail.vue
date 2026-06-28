<script setup lang="ts">
import { marked } from "marked";
import CopyButton from "../../../components/common/display/CopyButton.vue";
import { TEXT_EVENT_TOOL_RESULT, TEXT_EVENT_GUARDRAIL_BLOCKED } from "../../../constants/icons";
import {
  getRoleLabel,
  getRoleClass,
  getMessageClass,
} from "../../../domain/assistant";
import {
  getStepPayload,
  getMsgPayload,
  getToolCallPayload,
  getToolResPayload,
  getViolationPayload,
} from "../../../utils/dispatcher";
import type { AgentEvent } from "../../../types";

defineProps<{
  events: AgentEvent[];
}>();
</script>

<template>
  <div class="log-section">
    <h4 class="section-header section-header--accent">
      Execution Audit Trail (Terminal Log)
    </h4>
    <div class="terminal-box">
      <div v-for="(ev, i) in events" :key="i" class="event-line">
        <!-- Step Start -->
        <div v-if="ev.type === 'step_start'" class="event-step">
          <span class="step-label">Step {{ getStepPayload(ev).step }}</span>
        </div>

        <!-- Message/Error -->
        <div
          v-else-if="
            (ev.type === 'message' || ev.type === 'error') &&
            getMsgPayload(ev).content
          "
          class="event-message"
          :class="getMessageClass(getMsgPayload(ev).role, ev.type)"
        >
          <span
            class="role-label"
            :class="getRoleClass(getMsgPayload(ev).role)"
          >
            {{ getRoleLabel(getMsgPayload(ev).role) }}
          </span>

          <div
            v-if="getMsgPayload(ev).role === 'assistant'"
            class="message-text prose prose-invert prose-xs max-w-none"
            v-html="marked.parse(getMsgPayload(ev).content)"
          ></div>
          <p v-else class="message-text">{{ getMsgPayload(ev).content }}</p>
        </div>

        <!-- Tool Call (Start) -->
        <div v-else-if="ev.type === 'tool_call'" class="event-tool-call">
          <div class="tool-call-header">
            <span class="tool-icon">🛠️</span>
            <span class="tool-name"
              >Attempting {{ getToolCallPayload(ev).function.name }}...</span
            >
            <CopyButton
              :text="getToolCallPayload(ev).function.arguments"
              iconSize="sm"
              class="btn-copy-mini"
              title="Copy arguments"
            />
          </div>
          <pre class="tool-args">{{
            getToolCallPayload(ev).function.arguments
          }}</pre>
        </div>

        <!-- Tool Result -->
        <div v-else-if="ev.type === 'tool_result'" class="event-result">
          <details class="res-details">
            <summary class="res-summary">
              <span class="res-icon">{{ TEXT_EVENT_TOOL_RESULT }}</span>
              <span class="res-name"
                >{{ getToolResPayload(ev).name }} finished</span
              >
              <span class="res-hint">(click to view result)</span>
            </summary>
            <CopyButton
              :text="getToolResPayload(ev).result"
              iconSize="sm"
              class="btn-copy-mini result-copy-btn"
              title="Copy result"
            />
            <pre class="res-data">{{
              typeof getToolResPayload(ev).result === "string"
                ? getToolResPayload(ev).result
                : JSON.stringify(getToolResPayload(ev).result, null, 2)
            }}</pre>
          </details>
        </div>
        <!-- Guardrail Violation -->
        <div
          v-else-if="ev.type === 'guardrail_violation'"
          class="event-violation"
        >
          <div class="violation-header">
            <span class="violation-icon">{{ TEXT_EVENT_GUARDRAIL_BLOCKED }}</span>
            <span class="violation-title"
              >Guardrail Blocked:
              {{ getViolationPayload(ev).tool || "Unknown Tool" }}</span
            >
          </div>
          <div class="violation-body">{{ getViolationPayload(ev).error }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.log-section {
  @apply border-l-2 border-gray-700/30 pl-4 py-2;
}

.section-header {
  @apply text-[10px] font-black uppercase tracking-[0.2em] mb-4;
}

.section-header--accent {
  @apply text-purple-400/80;
}

.terminal-box {
  @apply bg-black/40 rounded-lg p-5 font-mono text-[11px] space-y-4 shadow-inner border border-white/5;
}

.event-line {
  @apply border-l-2 border-gray-800/50 pl-4 py-1;
}

.event-step {
  @apply border-l-blue-500/50;
}

.step-label {
  @apply text-blue-400/80 font-bold uppercase;
}

.event-message {
  @apply border-l-gray-800/50 my-2 pl-4 py-2;
}

.system-msg {
  @apply border-l-indigo-500/30 bg-indigo-500/5 !important;
}

.system-error-msg {
  @apply border-l-red-500/30 bg-red-500/10 !important;
}

.role-label {
  @apply block mb-1 uppercase tracking-tighter font-bold text-[10px];
}

.role-system {
  @apply text-indigo-400/80;
}

.role-assistant {
  @apply text-emerald-400/80;
}

.message-text {
  @apply text-gray-400 leading-relaxed;
}

/* Typography Overrides */
.message-text :deep(p) {
  @apply mb-2;
}
.message-text :deep(ul),
.message-text :deep(ol) {
  @apply mb-2 ml-4;
}

.event-tool-call {
  @apply border-l-blue-400/50 py-1;
}

.tool-call-header {
  @apply flex items-center gap-2 mb-1;
}

.tool-name {
  @apply text-blue-300/80 font-bold;
}

.btn-copy-mini {
  @apply ml-auto p-1.5 text-gray-500 hover:text-white transition-colors flex items-center justify-center;
}

.tool-args {
  @apply bg-blue-900/5 p-2 rounded text-[10px] text-blue-200/40 italic;
}

.event-result {
  @apply border-l-green-500/50;
}

.res-details {
  @apply relative;
}

.result-copy-btn {
  @apply absolute top-0 right-0 z-10 opacity-0 transition-opacity;
}
.res-details:hover .result-copy-btn {
  @apply opacity-100;
}

.res-summary {
  @apply flex items-center gap-2 text-green-400/80 cursor-pointer hover:text-green-300 transition-colors list-none outline-none;
}

.res-summary::-webkit-details-marker {
  display: none;
}

.res-hint {
  @apply text-[9px] text-gray-600 italic;
}

.res-data {
  @apply mt-2 bg-black/60 p-3 rounded text-[10px] text-gray-500 max-h-60 overflow-y-auto border border-white/5;
}

.event-violation {
  @apply border-l-red-500/50 bg-red-900/10 p-3 rounded;
}

.violation-header {
  @apply flex items-center gap-2 mb-1;
}

.violation-title {
  @apply text-red-400 font-bold uppercase tracking-tight;
}

.violation-body {
  @apply text-red-300/70 text-[11px] leading-relaxed;
}
</style>
