<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { useConfirm } from '../../../composables/ui/useConfirm'
import { asUtc, formatAbsoluteTime } from '../../../utils/format/time'
import { formatMemoryUsage } from '../../../utils/format/memoryUsage'
import { HOT_TAG, MEMORY_PRIORITY_LABEL, MEMORY_TYPE_LABEL, PRIORITY_NORMAL } from '../../../constants/memory'
import type { MemoryEntry, MemoryPriority } from '../../../types/memory'
import StatusTag from '../../common/display/StatusTag.vue'
import CopyButton from '../../common/display/CopyButton.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import BaseToggle from '../../common/buttons/BaseToggle.vue'

// One memory entry: read, copy, or edit its title and content.

const props = defineProps<{
  entry: MemoryEntry
  workspaceId: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated'): void
}>()

const { updateMemory, setHot, setPriority } = useMemory()
const { confirm } = useConfirm()

const HOT_LABEL = "Always in the model's context"
const HOT_HINT = 'Sent with every run, within the memory budget. Off: saved and searchable, used only when asked for.'

// A fact lives in the workspace it is stored in: user-wide facts are stored
// under "global" even while a workspace is open.
const storedIn = computed(() => props.entry.workspace_id || props.workspaceId)

const PRIORITY_HINT = 'Only matters for always-on facts: on a small context window, higher-priority facts are cut last.'
const PRIORITY_OPTIONS = (Object.keys(MEMORY_PRIORITY_LABEL) as unknown as MemoryPriority[]).map((value) => ({ value, label: MEMORY_PRIORITY_LABEL[value] }))
const priority = ref<MemoryPriority>(PRIORITY_NORMAL)
async function changePriority(event: Event) {
  const next = Number((event.target as HTMLSelectElement).value) as MemoryPriority
  if (await setPriority(props.entry, next)) {
    priority.value = next
    emit('updated')
  }
}

const hot = ref(false)
// Bumped to re-mount the switch when a change is declined or fails: the browser
// has already flipped the checkbox, and the bound value did not change.
const toggleKey = ref(0)
watch(() => props.entry, (entry) => {
  hot.value = (entry.tags ?? []).includes(HOT_TAG)
  priority.value = entry.priority ?? PRIORITY_NORMAL
}, { immediate: true })

async function changeHot(next: boolean) {
  if (!next) {
    const confirmed = await confirm({
      title: 'Stop sending this fact with every run?',
      message: `"${props.entry.title || 'Untitled'}" stays saved and searchable, but the model will only see it when it asks.`,
      type: 'warning',
      confirmText: 'Stop sending',
      cancelText: 'Keep sending',
    })
    if (!confirmed) {
      toggleKey.value++
      return
    }
  }
  if (await setHot(props.entry, next)) {
    hot.value = next
    emit('updated')
  } else {
    toggleKey.value++
  }
}

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
  await updateMemory(storedIn.value, props.entry.id, editTitle.value, editContent.value)
  isEditing.value = false
  emit('updated')
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
        <BaseButton v-if="!isEditing" variant="ghost" size="sm" icon="edit" @click="startEdit">Edit</BaseButton>
        <CopyButton :text="entry.content" title="Copy memory" />
        <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close" @click="emit('close')" />
      </span>
    </header>

    <div class="flex flex-col gap-1">
      <BaseToggle :key="toggleKey" :model-value="hot" :label="HOT_LABEL" @update:model-value="changeHot" />
      <p class="m-0 text-[length:var(--text-small)] text-faint">{{ HOT_HINT }}</p>
      <label class="mt-1 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-secondary">
        <span>Priority</span>
        <select
          aria-label="Priority"
          :value="priority"
          :disabled="!hot"
          class="h-8 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2"
          @change="changePriority"
        >
          <option v-for="opt in PRIORITY_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
      </label>
      <p class="m-0 text-[length:var(--text-small)] text-faint">{{ PRIORITY_HINT }}</p>
    </div>

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
      <span data-test="memory-usage">{{ formatMemoryUsage(entry) }}</span>
      <span v-if="entry.last_used_at">Last used {{ formatAbsoluteTime(asUtc(entry.last_used_at)) }}</span>
    </footer>
  </article>
</template>

<style scoped lang="postcss">
.field-control {
  @apply w-full rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-primary focus-visible:outline-none focus-visible:ring-2;
}
</style>
