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
</script>

<template>
  <div class="session-panel">
    <div class="session-panel-header">
      <h3 class="session-panel-title">Conversations</h3>
      <div class="flex items-center gap-1">
        <button
          v-if="props.sessions.length > 0"
          @click="emit('clear-all')"
          class="btn-clear-all"
          title="Delete all conversations"
        >
          <Icon name="trash" size="xs" />
        </button>
        <button @click="emit('new-chat')" class="btn-header-icon" title="New Chat">
          <Icon name="plus" size="sm" />
        </button>
        <button
          v-if="props.isMobile"
          @click="emit('close')"
          class="btn-header-icon"
          title="Close history"
        >
          <Icon name="close" size="sm" />
        </button>
      </div>
    </div>

    <div v-if="props.sessions.length === 0" class="empty-sessions">
      No history in this workspace.
    </div>

    <div class="session-list">
      <div
        v-for="session in props.sessions"
        :key="session.id"
        class="session-row"
        :class="{ 'session-row--active': currentSessionId === session.id }"
      >
        <button
          v-if="renaming !== session.id"
          @click="emit('load', session.id)"
          class="session-item"
        >
          <span class="session-snippet">{{ session.snippet || 'Empty conversation' }}</span>
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
  @apply flex flex-col h-full w-64 bg-gray-800 border border-white/5 overflow-hidden;
}
@media (max-width: 639px) {
  .session-panel { @apply fixed left-0 top-0 bottom-0 w-72 z-40; }
}

.session-panel-header {
  @apply flex items-center justify-between px-3 py-2.5 border-b border-white/5; }

.session-panel-title {
  @apply text-xs font-bold uppercase tracking-wider text-gray-400;
}

.btn-header-icon {
  @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-400 hover:text-white transition-colors flex items-center justify-center;
}

.btn-clear-all {
  @apply p-1 rounded-md hover:bg-red-500/15 text-gray-500 hover:text-red-400 transition-colors flex items-center justify-center;
}

.empty-sessions {
  @apply p-4 text-xs text-center text-gray-500 italic;
}

.session-list {
  @apply flex-1 overflow-y-auto;
}

.session-row {
  @apply relative flex items-center px-3 py-3 hover:bg-white/[0.03] transition-colors;
}

.session-row--active {
  @apply bg-blue-600/10;
}

.session-item {
  @apply flex flex-col gap-0.5 text-left min-w-0 flex-1 bg-transparent border-none cursor-pointer self-stretch justify-center;
}

.session-snippet {
  @apply text-sm text-gray-200 truncate font-medium block w-full;
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
  display: flex;
}

.btn-action-icon {
  @apply p-1 rounded hover:bg-gray-700/50 text-gray-500 hover:text-gray-300 transition-colors;
}

.btn-delete:hover {
  @apply hover:bg-red-500/15 hover:text-red-400;
}
</style>
