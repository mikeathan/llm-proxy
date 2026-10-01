<script setup lang="ts">
import { onUnmounted, ref } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { asUtc, formatAbsoluteTime } from '../../../utils/format/time'
import { MEMORY_TYPE_LABEL } from '../../../constants/memory'
import type { MemoryEntry } from '../../../types/memory'
import StatusTag from '../../common/display/StatusTag.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'

// One memory entry: read, copy, or edit its title and content.

const props = defineProps<{
  entry: MemoryEntry
  workspaceId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated'): void
}>()

const { updateMemory } = useMemory()

const COPY_FEEDBACK_MS = 2000

const isEditing = ref(false)
const editTitle = ref('')
const editContent = ref('')
const copied = ref('')
let copiedTimer: ReturnType<typeof setTimeout> | null = null
onUnmounted(() => {
  if (copiedTimer) clearTimeout(copiedTimer)
})

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

async function copyContent() {
  try {
    await navigator.clipboard.writeText(props.entry.content)
    copied.value = 'Copied'
  } catch {
    copied.value = 'Copy failed'
  }
  if (copiedTimer) clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copied.value = ''), COPY_FEEDBACK_MS)
}
</script>

<template>
  <article class="flex min-h-0 flex-col gap-3">
    <header class="flex flex-wrap items-center justify-between gap-2">
      <span class="flex items-center gap-2">
        <StatusTag state="neutral" :label="MEMORY_TYPE_LABEL[entry.memory_type] ?? entry.memory_type" />
        <span class="font-mono text-[length:var(--text-small)] text-muted">by {{ entry.source }}</span>
      </span>
      <span class="flex items-center gap-1">
        <span role="status" class="font-mono text-[length:var(--text-micro)] text-muted">{{ copied }}</span>
        <BaseButton v-if="!isEditing" variant="ghost" size="sm" icon="edit" @click="startEdit">Edit</BaseButton>
        <BaseButton variant="ghost" size="sm" @click="copyContent">Copy</BaseButton>
        <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close" @click="emit('close')" />
      </span>
    </header>

    <template v-if="!isEditing">
      <h2 class="m-0 text-[length:var(--text-heading)] font-semibold text-primary">{{ entry.title || 'Untitled' }}</h2>
      <p class="m-0 whitespace-pre-wrap break-words text-secondary">{{ entry.content }}</p>
    </template>
    <form v-else class="flex flex-col gap-2" @submit.prevent="saveEdit">
      <label class="flex flex-col gap-1.5">
        <span class="text-[length:var(--text-small)] font-medium text-secondary">Title</span>
        <input v-model="editTitle" class="field-control h-8" />
      </label>
      <label class="flex flex-col gap-1.5">
        <span class="text-[length:var(--text-small)] font-medium text-secondary">Content</span>
        <textarea v-model="editContent" rows="10" class="field-control py-2"></textarea>
      </label>
      <div class="flex justify-end gap-2">
        <BaseButton variant="secondary" @click="cancelEdit">Cancel</BaseButton>
        <BaseButton type="submit">Save</BaseButton>
      </div>
    </form>

    <footer class="flex flex-wrap gap-x-4 gap-y-1 font-mono text-[length:var(--text-micro)] text-faint">
      <span>Created {{ formatAbsoluteTime(asUtc(entry.created_at)) }}</span>
      <span v-if="entry.created_at !== entry.updated_at">Updated {{ formatAbsoluteTime(asUtc(entry.updated_at)) }}</span>
    </footer>
  </article>
</template>

<style scoped lang="postcss">
.field-control {
  @apply w-full rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-primary focus-visible:outline-none focus-visible:ring-2;
}
</style>
