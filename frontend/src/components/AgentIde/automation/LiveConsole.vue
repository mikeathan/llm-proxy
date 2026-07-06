<script setup lang="ts">
import { onMounted, onUnmounted, watch, computed, nextTick } from "vue";
import type { AgentEvent } from "../../../types/dispatcher";
import { formatEventsToText } from "../../../utils/dispatcher";
import CopyButton from "../../common/display/CopyButton.vue";

import { useLiveConsole } from "../../../composables/automation/useLiveConsole";
import { useAutoScroll } from "../../../composables/ui/useAutoScroll";
import TerminalOutput from "./TerminalOutput.vue";
import Icon from "../../../components/icons/Icon.vue";

const props = defineProps<{
  workspaceId: string;
  isActive: boolean;
  isExecuting?: boolean;
  historyEvents?: AgentEvent[];
}>();

const { container: scrollContainer, scrollToBottom, toggleScroll, scrollDirection, capturePosition, scrollIfNearBottom, updateWasNearBottom } = useAutoScroll();

const {
  liveEvents,
  displayEvents,
  isConnected,
  pendingDecision,
  connect,
  disconnect,
  clearEvents,
  submitDecision,
} = useLiveConsole(
  () => props.workspaceId,
  () => props.isExecuting,
  () => props.historyEvents,
);

onMounted(() => {
  connect();
  nextTick(() => scrollToBottom(undefined, "instant"));
});

onUnmounted(() => {
  disconnect();
});

watch(
  () => props.workspaceId,
  () => {
    clearEvents();
    connect();
  },
);

watch(
  () => props.isExecuting,
  (executing) => {
    if (executing) {
      nextTick(() => scrollToBottom(undefined, "instant"));
    }
  },
);

watch(
  displayEvents,
  () => {
    capturePosition();
    nextTick(() => {
      scrollIfNearBottom(undefined, "instant");
    });
  },
  { deep: true },
);

const fullTerminalText = computed(() =>
  formatEventsToText(displayEvents.value),
);
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
        <button
          v-if="displayEvents.length > 0"
          class="scroll-btn"
          :title="scrollDirection === 'down' ? 'Scroll to bottom' : 'Scroll to top'"
          @click="toggleScroll(scrollContainer)"
        >
          <Icon v-if="scrollDirection === 'down'" name="arrow-down" size="sm" />
          <Icon v-else name="arrow-up" size="sm" />
        </button>
        <CopyButton
          v-if="displayEvents.length > 0"
          :text="fullTerminalText"
          iconSize="sm"
          class="btn-clear-term"
        />
      </div>
    </div>

    <div ref="scrollContainer" class="terminal-scroll" @scroll="updateWasNearBottom()">
    <TerminalOutput
      :events="displayEvents"
      :workspaceId="props.workspaceId"
      :pendingDecision="pendingDecision"
      :scrollDirection="scrollDirection"
      @scroll-toggle="toggleScroll()"
      @allow-decision="(persist: boolean) => submitDecision(true, persist)"
      @deny-decision="() => submitDecision(false, false)"
    />
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

.scroll-btn {
  @apply text-gray-500 hover:text-white transition-all flex items-center justify-center p-1 rounded;
}

.scroll-btn:hover {
  @apply bg-gray-700/50;
}

.scroll-btn:active {
  @apply bg-gray-600/50;
}

.terminal-scroll {
  @apply flex-1 overflow-y-auto bg-[#0d1117];
}


</style>
