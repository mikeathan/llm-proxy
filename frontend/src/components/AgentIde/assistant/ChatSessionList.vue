<script setup lang="ts">
import { ref } from 'vue'
import type { SessionBrief } from '../../../types/assistant'
import { formatTime } from '../../../utils/format/time'
import Icon from '../../icons/Icon.vue'

const props = defineProps<{
  sessions: SessionBrief[]
  currentSessionId: string | null
  isMobile?: boolean
}>()

const emit = defineEmits<{
  (e: 'load', sessionId: string): void
  (e: 'delete', sessionId: string): void
  (e: 'rename', sessionId: string, title: string): void
  (e: 'cancel', sessionId: string): void
  (e: 'new-chat'): void
  (e: 'clear-all'): void
  (e: 'close'): void
}>()

const renaming = ref<string | null>(null)
const renameInput = ref('')

function startRename(sessionId: string, current: string) {
  renaming.value = sessionId
  renameInput.value = current
}

function confirmRename() {
  if (renaming.value && renameInput.value.trim()) {
    emit('rename', renaming.value, renameInput.value.trim())
  }
  renaming.value = null
}

function cancelRename() {
  renaming.value = null
}

const sourceIcon = (sessionId: string): string => {
  if (sessionId.startsWith('wb_telegram')) return '📞'
  if (sessionId.startsWith('wb_')) return '📡'
  return ''
}
</script>

<template>
  <div class="session-panel" :class="{ 'session-panel--mobile': isMobile }">
    <div class="session-panel-header">
      <h3 class="session-panel-title">Conversations</h3>
      <div class="header-actions">
        <button
          v-if="sessions.length > 0"
          @click="emit('clear-all')"
          class="btn-header-icon"
          title="Delete all conversations"
        >
          <Icon name="trash" size="xs" />
        </button>
        <button @click="emit('new-chat')" class="btn-header-icon" title="New Chat">
          <Icon name="plus" size="sm" />
        </button>
        <button
          v-if="isMobile"
          @click="emit('close')"
          class="btn-header-icon btn-close"
          title="Close"
        >
          <Icon name="close" size="sm" />
        </button>
      </div>
    </div>

    <div v-if="sessions.length === 0" class="empty-sessions">
      No history in this workspace.
    </div>

    <div class="session-list">
      <div
        v-for="session in sessions"
        :key="session.id"
        class="session-row"
        :class="{ 'session-row--active': currentSessionId === session.id }"
      >
        <button
          v-if="renaming !== session.id"
          @click="emit('load', session.id)"
          class="session-item"
        >
          <div class="session-item-row">
            <span class="session-source">{{ sourceIcon(session.id) }}</span>
            <span v-if="session.running" class="session-dot" title="Running">●</span>
            <span class="session-snippet">{{ session.snippet || 'Empty conversation' }}</span>
          </div>
          <span class="session-time">{{ formatTime(session.updated_at || '') }}</span>
        </button>

        <div v-else class="rename-form">
          <input
            v-model="renameInput"
            @keydown.enter="confirmRename"
            @keydown.escape="cancelRename"
            @blur="confirmRename"
            class="rename-input"
            autofocus
          />
        </div>

        <div v-if="renaming !== session.id" class="session-actions">
          <button
            v-if="session.running"
            @click.stop="emit('cancel', session.id)"
            class="btn-action-icon btn-cancel"
            title="Cancel run"
          >
            <Icon name="close" size="xs" />
          </button>
          <button
            @click.stop="startRename(session.id, session.snippet)"
            class="btn-action-icon"
            title="Rename"
          >
            <Icon name="edit" size="xs" />
          </button>
          <button
            @click.stop="emit('delete', session.id)"
            class="btn-action-icon btn-delete"
            title="Delete"
          >
            <Icon name="trash" size="xs" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.session-panel {
  @apply h-full w-full flex flex-col bg-gray-800/80 overflow-hidden;
}

.session-panel--mobile {
  @apply bg-gray-800;
}

.session-panel-header {
  @apply flex items-center justify-between px-3 py-2.5 border-b border-white/5 shrink-0;
}

.session-panel-title {
  @apply text-xs font-bold uppercase tracking-wider text-gray-400;
}

.header-actions {
  @apply flex items-center gap-1;
}

.btn-header-icon {
  @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-400 hover:text-white transition-all duration-150 flex items-center justify-center;
}
.btn-header-icon:active { @apply scale-95; }

.btn-close {
  @apply hover:bg-red-500/15 hover:text-red-400;
}

.empty-sessions {
  @apply p-4 text-xs text-center text-gray-500 italic;
}

.session-list {
  @apply flex-1 overflow-y-auto;
}

.session-row {
  @apply relative flex items-center px-3 py-2.5 cursor-pointer transition-all duration-150;
  border-left: 3px solid transparent;
}

.session-row:hover {
  @apply bg-white/[0.04];
}

.session-row--active {
  background-color: rgba(59, 130, 246, 0.08);
  border-left-color: rgb(59, 130, 246);
}

.session-item {
  @apply flex flex-col gap-0.5 text-left min-w-0 flex-1 bg-transparent border-none cursor-pointer self-stretch justify-center;
}

.session-item-row {
  @apply flex items-center gap-1.5 min-w-0;
}

.session-source {
  @apply text-[11px] shrink-0;
}

.session-dot {
  @apply text-blue-500 text-[10px] shrink-0 animate-pulse;
}

.session-snippet {
  @apply text-sm text-gray-200 truncate font-medium min-w-0;
}

.session-time {
  @apply text-[10px] text-gray-500 font-mono;
}

.rename-form {
  @apply flex-1;
}

.rename-input {
  @apply w-full bg-gray-900 border border-blue-500 rounded px-2 py-1 text-sm text-gray-200 outline-none;
}

.session-actions {
  @apply hidden gap-0.5 ml-2 shrink-0;
}
.session-row:hover .session-actions {
  @apply flex;
}

.btn-action-icon {
  @apply p-1 rounded hover:bg-gray-700/50 text-gray-500 hover:text-gray-300 transition-colors;
}

.btn-cancel {
  @apply text-orange-400;
}
.btn-cancel:hover {
  @apply bg-orange-500/15 text-orange-300;
}

.btn-delete:hover {
  @apply hover:bg-red-500/15 hover:text-red-400;
}
</style>
