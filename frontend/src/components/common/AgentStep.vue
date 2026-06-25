<script setup lang="ts">
import { ref, computed } from 'vue'
import type { AgentStepData } from '../../types/agent-run'
import { formatDuration, truncatePreview } from '../../utils/turns'
import CopyButton from './CopyButton.vue'
import Icon from '../icons/Icon.vue'

const props = defineProps<{
  step: AgentStepData
}>()

const expanded = ref(false)

const iconClass = computed(() => {
  switch (props.step.status) {
    case 'running': return 'step-icon--running'
    case 'success': return 'step-icon--success'
    case 'error': return 'step-icon--error'
  }
})

const previewText = computed(() => {
  const text = props.step.error || props.step.result
  if (!text) return ''
  return truncatePreview(text)
})
</script>

<template>
  <div
    class="agent-step"
    :class="[`agent-step--${step.status}`, { 'agent-step--expanded': expanded }]"
    @click="expanded = !expanded"
  >
    <div class="step-header">
      <span class="step-indicator" :class="iconClass">
        <span v-if="step.status === 'running'" class="step-spinner"></span>
        <Icon v-else-if="step.status === 'success'" name="check" size="xs" />
        <Icon v-else name="close" size="xs" />
      </span>
      <span class="step-tool-name">{{ step.toolName }}</span>
      <span class="step-preview">{{ previewText }}</span>
      <span v-if="step.durationMs > 0" class="step-duration">{{ formatDuration(step.durationMs) }}</span>
      <span class="step-chevron" :class="{ 'step-chevron--open': expanded }">
        <Icon name="chevron-right" size="xs" />
      </span>
    </div>
    <div v-if="expanded" class="step-body" @click.stop>
      <div class="step-section">
        <div class="step-section-label">Arguments</div>
        <pre class="step-code">{{ step.args }}</pre>
      </div>
      <div v-if="step.result" class="step-section">
        <div class="step-section-label">
          {{ step.error ? 'Error' : 'Result' }}
          <CopyButton :text="step.error || step.result" class="btn-copy-step" />
        </div>
        <pre class="step-code" :class="{ 'step-code--error': !!step.error }">{{ step.error || step.result }}</pre>
      </div>
    </div>
  </div>
</template>

<style scoped>
.agent-step {
  border: 1px solid var(--step-border, #2a2a4a);
  border-radius: var(--step-radius, 8px);
  background: var(--step-bg, #16213e);
  cursor: pointer;
  transition: border-color var(--transition-fast, 150ms ease), background var(--transition-fast, 150ms ease);
  overflow: hidden;
}

.agent-step:hover {
  background: var(--step-hover-bg, #1c2a4a);
  border-color: var(--step-border, #3a3a5a);
}

.agent-step--expanded {
  border-color: var(--color-info, #3b82f6);
}

.agent-step--error {
  border-left: 3px solid var(--color-error, #ef4444);
}

.agent-step--success {
  border-left: 3px solid var(--color-success, #22c55e);
}

.agent-step--running {
  border-left: 3px solid var(--color-running, #f59e0b);
}

.step-header {
  display: flex;
  align-items: center;
  gap: var(--gap-md, 8px);
  padding: 8px 12px;
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  user-select: none;
}

.step-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

.step-indicator.step-icon--success { color: var(--color-success, #22c55e); }
.step-indicator.step-icon--error { color: var(--color-error, #ef4444); }
.step-indicator.step-icon--running { color: var(--color-running, #f59e0b); }

.step-spinner {
  width: 10px;
  height: 10px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 600ms linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.step-tool-name {
  font-weight: 600;
  color: var(--color-text, #e2e8f0);
  white-space: nowrap;
  flex-shrink: 0;
}

.step-preview {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-muted, #94a3b8);
  font-size: 11px;
}

.step-duration {
  color: var(--color-text-dim, #64748b);
  font-size: 10px;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.step-chevron {
  display: flex;
  align-items: center;
  color: var(--color-text-dim, #64748b);
  transition: transform var(--transition-fast, 150ms ease);
  flex-shrink: 0;
}

.step-chevron--open {
  transform: rotate(90deg);
}

.step-body {
  border-top: 1px solid var(--step-border, #2a2a4a);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  cursor: default;
}

.step-section-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-dim, #64748b);
  margin-bottom: 6px;
}

.step-code {
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 6px;
  padding: 10px 12px;
  font-family: var(--font-mono, monospace);
  font-size: 11px;
  line-height: 1.5;
  color: var(--color-text-muted, #94a3b8);
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 400px;
  overflow-y: auto;
  margin: 0;
}

.step-code--error {
  color: var(--color-error, #ef4444);
}

.btn-copy-step {
  display: inline-flex;
  opacity: 0.6;
  transition: opacity var(--transition-fast, 150ms ease);
}

.btn-copy-step:hover {
  opacity: 1;
}
</style>
