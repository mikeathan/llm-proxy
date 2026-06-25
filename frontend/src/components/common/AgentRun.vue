<script setup lang="ts">
import { computed } from 'vue'
import type { AgentTurnData } from '../../types/agent-run'
import { formatDuration } from '../../utils/turns'
import AgentThinking from './AgentThinking.vue'
import AgentStep from './AgentStep.vue'

const props = defineProps<{
  turn: AgentTurnData
}>()

const totalTime = computed(() => {
  let total = props.turn.totalDurationMs
  if (total <= 0 && props.turn.steps.length > 0) {
    total = props.turn.steps.reduce((sum, s) => sum + s.durationMs, 0)
  }
  return total
})

const shouldAutoCollapse = computed(() => props.turn.state === 'completed')
</script>

<template>
  <div class="agent-run" :class="{ 'agent-run--streaming': turn.state === 'streaming' }">
    <div class="agent-run-header">
      <div class="header-left">
        <span v-if="turn.state === 'streaming'" class="header-spinner"></span>
        <span class="header-label">{{ turn.state === 'streaming' ? 'Working…' : 'Agent Run' }}</span>
      </div>
      <div class="header-right">
        <span v-if="totalTime > 0" class="header-duration">{{ formatDuration(totalTime) }}</span>
      </div>
    </div>

    <div class="agent-run-body">
      <AgentThinking
        v-if="turn.thinking"
        :content="turn.thinking"
        :auto-collapse="shouldAutoCollapse && turn.thinking.length > 0"
      />

      <div v-if="turn.steps.length > 0" class="steps-list">
        <AgentStep
          v-for="(step, i) in turn.steps"
          :key="'s-' + i"
          :step="step"
        />
      </div>

      <div v-if="turn.state === 'streaming'" class="streaming-indicator">
        <span class="streaming-dot"></span>
        Generating response…
      </div>
    </div>
  </div>
</template>

<style scoped>
.agent-run {
  border: 1px solid var(--agent-run-border, #2d2d4a);
  border-radius: var(--agent-run-radius, 12px);
  background: var(--agent-run-bg, #1a1a2e);
  box-shadow: var(--agent-run-shadow, 0 4px 24px rgba(0, 0, 0, 0.3));
  overflow: hidden;
  width: 100%;
}

.agent-run--streaming {
  border-color: var(--color-running, #f59e0b);
}

.agent-run-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: var(--agent-run-header-bg, #16162a);
  border-bottom: 1px solid var(--agent-run-border, #2d2d4a);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-spinner {
  width: 12px;
  height: 12px;
  border: 2px solid var(--color-running, #f59e0b);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 600ms linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.header-label {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-dim, #64748b);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-duration {
  font-size: 10px;
  font-family: var(--font-mono, monospace);
  color: var(--color-text-dim, #64748b);
  font-variant-numeric: tabular-nums;
}

.agent-run-body {
  padding: 12px 16px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.steps-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.streaming-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  font-size: 11px;
  color: var(--color-text-dim, #64748b);
  font-family: var(--font-mono, monospace);
}

.streaming-dot {
  width: 6px;
  height: 6px;
  background: var(--color-running, #f59e0b);
  border-radius: 50%;
  animation: pulse-dot 1.2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 0.4; transform: scale(0.8); }
  50% { opacity: 1; transform: scale(1.2); }
}
</style>
