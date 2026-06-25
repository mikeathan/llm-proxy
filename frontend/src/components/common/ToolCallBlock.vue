<script setup lang="ts">
import CopyButton from "./CopyButton.vue";
import { TEXT_EVENT_TOOL_CALL } from "../../constants/icons";

const props = defineProps<{
  name: string;
  args: string;
}>();
</script>

<template>
  <div class="tool-call-block">
    <div class="tool-run-header">
      <span class="tool-icon">{{ TEXT_EVENT_TOOL_CALL }}</span>
      <span class="tool-name">Executing {{ name }}...</span>
      <CopyButton :text="args" iconSize="sm" class="btn-copy-mini" />
    </div>
    <pre class="tool-args">{{ args }}</pre>
  </div>
</template>

<style scoped lang="postcss">
.tool-call-block {
  @apply my-2;
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
</style>
