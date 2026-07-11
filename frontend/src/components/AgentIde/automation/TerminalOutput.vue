<script setup lang="ts">
import type { AgentEvent } from "../../../types/dispatcher"
import {
  getRoleLabel,
  getMessageClass,
  getRoleClass,
  getContentClass,
} from "../../../domain/assistant"
import {
  getStepPayload,
  getMsgPayload,
  getToolCallPayload,
  getToolResPayload,
  getViolationPayload,
} from "../../../utils/dispatcher"
import { marked } from "marked"
import ToolCallBlock from "../../common/chat/ToolCallBlock.vue"
import ToolResultBlock from "../../common/chat/ToolResultBlock.vue"
import GuardrailBanner from "../../common/chat/GuardrailBanner.vue"
import LifecycleMessage from "../../common/chat/LifecycleMessage.vue"

defineProps<{
  events: AgentEvent[]
  workspaceId: string
  pendingDecision: any
  scrollDirection: "up" | "down"
}>()

const emit = defineEmits<{
  (e: "scroll-toggle"): void
  (e: "allow-decision", persist: boolean): void
  (e: "deny-decision"): void
}>()

const formatTime = (ts?: string) => {
  if (!ts) return ""
  const date = new Date(ts)
  return date.toLocaleTimeString([], {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })
}
</script>

<template>
  <GuardrailBanner
    v-if="pendingDecision"
    :decision="pendingDecision"
    @allow="(persist: boolean) => emit('allow-decision', persist)"
    @deny="() => emit('deny-decision')"
  />

  <div class="terminal-body">
    <div v-if="events.length === 0" class="term-empty">
      Waiting for activity in {{ workspaceId }}...
    </div>

    <div class="term-content">
      <div v-for="ev in events" :key="(ev as any).id" class="term-line">
        <div v-if="ev.type === 'step_start'" class="line-step">
          <span class="step-label">Step {{ getStepPayload(ev).step }}</span>
          <span v-if="ev.timestamp" class="step-ts"
            >[{{ formatTime(ev.timestamp) }}]</span
          >
        </div>

        <div
          v-else-if="
            (ev.type === 'message' || ev.type === 'error') &&
            getMsgPayload(ev).content
          "
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

        <ToolCallBlock
          v-else-if="ev.type === 'tool_call'"
          :name="getToolCallPayload(ev).function.name"
          :args="getToolCallPayload(ev).function.arguments"
        />

        <ToolResultBlock
          v-else-if="ev.type === 'tool_result'"
          :name="getToolResPayload(ev).name"
          :result="getToolResPayload(ev).result"
          :error="getToolResPayload(ev).error"
        />

        <LifecycleMessage
          v-else-if="ev.type === 'lifecycle'"
          :phase="(ev.payload as any).phase"
          :payload="(ev.payload as any)"
        />

        <div
          v-else-if="ev.type === 'guardrail_violation'"
          class="line-violation"
        >
          <div class="violation-header">
            <span class="violation-icon">🛑</span>
            <span class="violation-title"
              >Guardrail Blocked: {{ getViolationPayload(ev).tool }}</span
            >
          </div>
          <div class="violation-body">
            {{ getViolationPayload(ev).error }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.terminal-body {
  @apply p-4 leading-relaxed;
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

.line-step {
  @apply mt-4 mb-2;
}

.step-label {
  @apply text-blue-400 font-bold uppercase;
}

.step-ts {
  @apply text-[10px] text-gray-600 ml-2 font-normal;
}

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

.msg-content :deep(p) {
  @apply mb-1 mt-0;
}
.msg-content :deep(h1),
.msg-content :deep(h2),
.msg-content :deep(h3) {
  @apply text-gray-100 font-bold mt-2 mb-1;
}
.msg-content :deep(h3) {
  @apply text-[11px];
}
.msg-content :deep(h2) {
  @apply text-[12px];
}
.msg-content :deep(h1) {
  @apply text-[14px];
}
.msg-content :deep(ul),
.msg-content :deep(ol) {
  @apply mb-1 ml-4;
}
.msg-content :deep(table) {
  @apply my-2 border-collapse text-[10px] w-full;
}
.msg-content :deep(th),
.msg-content :deep(td) {
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
