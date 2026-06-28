<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'

const props = defineProps<{
  content: string
  autoCollapse?: boolean
}>()

const collapsed = ref(true)

onMounted(() => {
  if (props.content && !props.autoCollapse) {
    collapsed.value = false
  }
})

watch(() => props.content, () => {
  if (props.content && !props.autoCollapse) {
    collapsed.value = false
  }
})

watch(() => props.autoCollapse, (val) => {
  if (val) {
    collapsed.value = true
  }
})
</script>

<template>
  <div v-if="content" class="agent-thinking" :class="{ 'agent-thinking--collapsed': collapsed }">
    <button class="thinking-toggle" @click="collapsed = !collapsed">
      <span class="thinking-icon">
        <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
             :class="{ 'rotate-90': !collapsed }" class="chevron-icon">
          <polyline points="6 4 10 8 6 12"></polyline>
        </svg>
      </span>
      <span class="thinking-label">Thinking</span>
      <span v-if="collapsed" class="thinking-snippet">{{ content.slice(0, 80) }}{{ content.length > 80 ? '…' : '' }}</span>
    </button>
    <div v-if="!collapsed" class="thinking-body">
      <div class="thinking-text">{{ content }}</div>
    </div>
  </div>
</template>

<style scoped>
.agent-thinking {
  border: 1px solid var(--step-border, #2a2a4a);
  border-radius: var(--step-radius, 8px);
  overflow: hidden;
  transition: border-color var(--transition-fast, 150ms ease);
  border-left: 3px solid var(--color-info, #3b82f6);
}

.thinking-toggle {
  display: flex;
  align-items: center;
  gap: var(--gap-md, 8px);
  width: 100%;
  padding: 8px 12px;
  background: transparent;
  border: none;
  color: var(--color-text, #e2e8f0);
  cursor: pointer;
  font-family: var(--font-sans, sans-serif);
  font-size: 12px;
  text-align: left;
  transition: background var(--transition-fast, 150ms ease);
  user-select: none;
}

.thinking-toggle:hover {
  background: rgba(255, 255, 255, 0.03);
}

.thinking-icon {
  display: flex;
  align-items: center;
  color: var(--color-info, #3b82f6);
  flex-shrink: 0;
}

.chevron-icon {
  width: 14px;
  height: 14px;
  transition: transform var(--transition-normal, 250ms ease);
}

.rotate-90 {
  transform: rotate(90deg);
}

.thinking-label {
  font-weight: 600;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-info, #3b82f6);
  flex-shrink: 0;
}

.thinking-snippet {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-dim, #64748b);
  font-size: 11px;
  font-family: var(--font-mono, monospace);
}

.thinking-body {
  border-top: 1px solid var(--step-border, #2a2a4a);
  padding: 12px;
  animation: slideDown 200ms ease;
}

@keyframes slideDown {
  from { opacity: 0; transform: translateY(-8px); }
  to { opacity: 1; transform: translateY(0); }
}

.thinking-text {
  font-size: 13px;
  line-height: 1.6;
  color: var(--color-text-muted, #94a3b8);
  white-space: pre-wrap;
}
</style>
