<script setup lang="ts">
import { ref } from 'vue'
import { useMemory } from '../../../composables/useMemory'
import type { MemoryEntry } from '../../../types/memory'

const props = defineProps<{
  entry: MemoryEntry
  workspaceId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated'): void
}>()

const {
  updateMemory,
} = useMemory()

const isEditing = ref(false)
const editTitle = ref('')
const editContent = ref('')

function startEdit() {
  editTitle.value = props.entry.title
  editContent.value = props.entry.content
  isEditing.value = true
}

function cancelEdit() {
  isEditing.value = false
}

async function saveEdit() {
  await updateMemory(props.workspaceId, props.entry.id, editTitle.value, editContent.value)
  isEditing.value = false
  emit('updated')
}

function formatTime(ts: string): string {
  if (!ts) return ''
  const d = new Date(ts.endsWith('Z') ? ts : ts + 'Z')
  return d.toLocaleString()
}

function typeLabel(t: string): string {
  switch (t) {
    case 'long_term': return 'Permanent'
    case 'daily': return 'Daily'
    case 'session': return 'Session'
    default: return t
  }
}

function copyContent() {
  navigator.clipboard.writeText(props.entry.content)
}
</script>

<template>
  <div class="detail-container">
    <!-- Header -->
    <div class="detail-header">
      <div class="detail-header-left">
        <span class="detail-type-badge" :class="'detail-type-badge--' + entry.memory_type">
          {{ typeLabel(entry.memory_type) }}
        </span>
        <span class="detail-source">by {{ entry.source }}</span>
      </div>
      <div class="detail-header-right">
        <button v-if="!isEditing" @click="startEdit" class="btn-icon" title="Edit">✏️</button>
        <button @click="copyContent" class="btn-icon" title="Copy content">📋</button>
        <button @click="emit('close')" class="btn-icon" title="Close">✕</button>
      </div>
    </div>

    <!-- Title -->
    <div v-if="!isEditing" class="detail-title">{{ entry.title || 'Untitled' }}</div>
    <input
      v-else
      v-model="editTitle"
      class="edit-input"
      placeholder="Title"
    />

    <!-- Content -->
    <div v-if="!isEditing" class="detail-content">{{ entry.content }}</div>
    <textarea
      v-else
      v-model="editContent"
      class="edit-textarea"
      placeholder="Content"
      rows="10"
    ></textarea>

    <!-- Edit actions -->
    <div v-if="isEditing" class="edit-actions">
      <button @click="saveEdit" class="btn-save">Save</button>
      <button @click="cancelEdit" class="btn-cancel">Cancel</button>
    </div>

    <!-- Footer -->
    <div class="detail-footer">
      <span class="detail-time">Created: {{ formatTime(entry.created_at) }}</span>
      <span v-if="entry.created_at !== entry.updated_at" class="detail-time">Updated: {{ formatTime(entry.updated_at) }}</span>
    </div>
  </div>
</template>

<style scoped>
.detail-container { @apply flex flex-col h-full p-4; }
.detail-header { @apply flex items-center justify-between mb-3; }
.detail-header-left { @apply flex items-center gap-2; }
.detail-header-right { @apply flex items-center gap-1; }
.detail-type-badge { @apply text-xs px-2 py-0.5 rounded font-mono; }
.detail-type-badge--long_term { @apply bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300; }
.detail-type-badge--daily { @apply bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300; }
.detail-type-badge--session { @apply bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300; }
.detail-source { @apply text-xs text-gray-400; }
.btn-icon { @apply px-1.5 py-0.5 text-xs rounded hover:bg-gray-200 dark:hover:bg-gray-600; }
.detail-title { @apply text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2; }
.edit-input { @apply w-full px-2 py-1 mb-2 text-lg font-semibold border border-gray-300 dark:border-gray-600 rounded bg-transparent; }
.detail-content { @apply flex-1 overflow-y-auto text-sm text-gray-700 dark:text-gray-300 whitespace-pre-wrap; }
.edit-textarea { @apply flex-1 w-full px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-transparent resize-none font-mono; }
.edit-actions { @apply flex gap-2 mt-2; }
.btn-save { @apply px-3 py-1 text-sm bg-blue-500 text-white rounded hover:bg-blue-600; }
.btn-cancel { @apply px-3 py-1 text-sm bg-gray-200 dark:bg-gray-700 rounded hover:bg-gray-300 dark:hover:bg-gray-600; }
.detail-footer { @apply mt-4 flex flex-col gap-0.5; }
.detail-time { @apply text-[11px] text-gray-400; }
</style>
