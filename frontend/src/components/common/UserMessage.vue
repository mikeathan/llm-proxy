<script setup lang="ts">
import CopyButton from './CopyButton.vue'
import Icon from '../icons/Icon.vue'

defineProps<{
  content: string
  timestamp?: string
}>()

const emit = defineEmits<{
  retry: []
}>()
</script>

<template>
  <div class="user-message">
    <div class="user-bubble">
      <div class="user-content">{{ content }}</div>
      <div class="user-meta">
        <div class="user-actions">
          <CopyButton :text="content" class="action-btn" title="Copy message" />
          <button class="action-btn action-btn--retry" title="Retry" @click="emit('retry')">
            <Icon name="refresh" size="sm" />
          </button>
        </div>
        <span v-if="timestamp" class="user-timestamp">{{ timestamp }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.user-message {
  display: flex;
  justify-content: flex-end;
  width: 100%;
}

.user-bubble {
  max-width: 85%;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-radius: 16px 16px 4px 16px;
  padding: 12px 16px;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.2);
}

.user-content {
  font-size: 14px;
  line-height: 1.6;
  color: #fff;
  white-space: pre-wrap;
  word-break: break-word;
}

.user-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
  gap: 8px;
}

.user-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity var(--transition-fast, 150ms ease);
}

.user-bubble:hover .user-actions {
  opacity: 1;
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  transition: background var(--transition-fast, 150ms ease), color var(--transition-fast, 150ms ease);
  padding: 0;
}

.action-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.action-btn--retry:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fbbf24;
}

.user-timestamp {
  font-size: 10px;
  font-family: var(--font-mono, monospace);
  color: rgba(255, 255, 255, 0.4);
  font-variant-numeric: tabular-nums;
}
</style>
