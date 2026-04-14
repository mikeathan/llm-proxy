<script setup lang="ts">
import { ref } from "vue";
import type { AgentEvent } from "../../../types/dispatcher";
import CopyButton from "../../common/CopyButton.vue";
import { 
  getStepPayload, 
  getMsgPayload, 
  getToolCallPayload, 
  getToolResPayload 
} from "../../../utils/dispatcher";

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


        <!-- Message -->
        <div
          v-else-if="ev.type === 'message' && getMsgPayload(ev).content"
          class="event-message"
        >
          <span class="role-label">Assistant:</span>
          <p class="message-text">{{ getMsgPayload(ev).content }}</p>
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
          <pre class="tool-args">{{ getToolCallPayload(ev).function.arguments }}</pre>
        </div>


        <!-- Tool Result -->
        <div v-else-if="ev.type === 'tool_result'" class="event-result">
          <details class="res-details">
            <summary class="res-summary">
              <span class="res-icon">✅</span>
              <span class="res-name">{{ getToolResPayload(ev).name }} finished</span>
              <CopyButton 
                :text="getToolResPayload(ev).result"
                iconSize="sm"
                class="btn-copy-mini"
                title="Copy result"
              />
              <span class="res-hint">(click to view result)</span>
            </summary>
            <pre class="res-data">{{ 
              typeof getToolResPayload(ev).result === 'string' 
                ? getToolResPayload(ev).result 
                : JSON.stringify(getToolResPayload(ev).result, null, 2) 
            }}</pre>
          </details>
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
  @apply border-l-purple-500/50;
}

.role-label {
  @apply text-purple-500/80 font-bold block mb-1 uppercase tracking-tighter;
}

.message-text {
  @apply text-gray-400 whitespace-pre-wrap leading-relaxed;
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
</style>
