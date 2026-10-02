<script setup lang="ts">
import MarkdownViewer from "../../../components/common/display/MarkdownViewer.vue";
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

          <MarkdownViewer
            v-if="getMsgPayload(ev).role === 'assistant'"
            class="message-text"
            variant="compact"
            :content="getMsgPayload(ev).content"
          />
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
  @apply border-l-2 border-hairline pl-4 py-2;
}

.section-header {
  @apply font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] mb-4;
}

.section-header--accent {
  @apply text-muted;
}

.terminal-box {
  @apply bg-canvas rounded-[var(--radius-sm)] p-5 font-mono text-[11px] space-y-4 border border-hairline;
}

.event-line {
  @apply border-l-2 border-hairline pl-4 py-1;
}

.event-step {
  @apply border-l-accent-info/50;
}

.step-label {
  @apply text-accent-info-text font-bold uppercase;
}

.event-message {
  @apply border-l-hairline my-2 pl-4 py-2;
}

.system-msg {
  @apply border-l-accent-info/30 bg-accent-info/[0.05] !important;
}

.system-error-msg {
  @apply border-l-state-error/30 bg-state-error/10 !important;
}

.role-label {
  @apply block mb-1 uppercase tracking-tighter font-bold text-[10px];
}

.role-system {
  @apply text-accent-info-text;
}

.role-assistant {
  @apply text-state-success;
}

.message-text {
  @apply text-secondary leading-relaxed;
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
  @apply border-l-accent-info/50 py-1;
}

.tool-call-header {
  @apply flex items-center gap-2 mb-1;
}

.tool-name {
  @apply text-accent-info-text font-bold;
}

.btn-copy-mini {
  @apply ml-auto;
}

.tool-args {
  @apply m-0 bg-surface-raised p-2 rounded-[2px] text-[10px] text-muted whitespace-pre-wrap break-words;
}

.event-result {
  @apply border-l-state-success/50;
}

.res-details {
  @apply relative;
}

/* Shown on hover and on keyboard focus (not hover only). */
.result-copy-btn {
  @apply absolute top-0 right-0 z-10 opacity-0 transition-opacity focus-within:opacity-100;
}
.res-details:hover .result-copy-btn {
  @apply opacity-100;
}

.res-summary {
  @apply flex items-center gap-2 rounded-[2px] text-state-success cursor-pointer list-none transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}

.res-summary::-webkit-details-marker {
  display: none;
}

.res-hint {
  @apply text-[9px] text-faint;
}

.res-data {
  @apply mt-2 bg-surface-raised p-3 rounded-[2px] text-[10px] text-muted max-h-60 overflow-y-auto border border-hairline;
}

.event-violation {
  @apply border-l-state-error/50 bg-state-error/[0.08] p-3 rounded-[2px];
}

.violation-header {
  @apply flex items-center gap-2 mb-1;
}

.violation-title {
  @apply text-state-error font-bold uppercase tracking-tight;
}

.violation-body {
  @apply text-secondary text-[11px] leading-relaxed;
}
</style>
