<script setup lang="ts">
import { computed } from "vue";
import CopyButton from "./CopyButton.vue";
import { TEXT_EVENT_TOOL_RESULT } from "../../constants/icons";

const props = defineProps<{
  name: string;
  result: string | object;
  error?: string;
}>();

const resultText = computed(() =>
  typeof props.result === "string"
    ? props.result
    : JSON.stringify(props.result, null, 2),
);
</script>

<template>
  <div class="tool-result-block">
    <details class="res-details">
      <summary class="res-summary">
        <span class="res-icon">{{ TEXT_EVENT_TOOL_RESULT }}</span>
        <span class="res-name">{{ name }} finished</span>
        <span class="res-hint ml-1">(click to view)</span>
      </summary>
      <CopyButton :text="resultText" class="btn-copy-mini summary-copy-btn" />
      <pre class="res-body">{{ resultText }}</pre>
    </details>
  </div>
</template>

<style scoped lang="postcss">
.tool-result-block {
  @apply my-2 mb-4;
}

.res-details {
  @apply cursor-pointer outline-none relative;
}

.summary-copy-btn {
  @apply absolute top-0 right-0 z-10 opacity-0 transition-opacity;
}
.res-details:hover .summary-copy-btn {
  @apply opacity-100;
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

.btn-copy-mini {
  @apply p-1 text-gray-500 hover:text-gray-300 transition-colors ml-1 focus:outline-none;
}

.res-body {
  @apply bg-[#161b22] border border-gray-800 rounded p-3 mt-2 text-[11px] text-green-500/80 overflow-y-auto max-h-80 whitespace-pre-wrap;
}
</style>
