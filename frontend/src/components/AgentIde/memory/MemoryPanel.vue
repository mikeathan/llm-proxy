<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { useConfirm } from '../../../composables/ui/useConfirm'
import { asUtc, formatAbsoluteTime, formatRelativeTime } from '../../../utils/format/time'
import { formatMemoryUsage } from '../../../utils/format/memoryUsage'
import { GLOBAL_MEMORY_WORKSPACE, HOT_TAG, MEMORY_TYPE_LABEL, PRIORITY_HIGH } from '../../../constants/memory'
import type { MemoryEntry } from '../../../types/memory'
import type { ChoiceOption } from '../../../types/ui'
import SegmentedControl from '../../common/forms/SegmentedControl.vue'
import SearchInput from '../../common/forms/SearchInput.vue'
import StatusTag from '../../common/display/StatusTag.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import LoadingState from '../../common/feedback/LoadingState.vue'
import EmptyState from '../../common/feedback/EmptyState.vue'
import MemoryAddForm from './MemoryAddForm.vue'
import MemoryImportExport from './MemoryImportExport.vue'
import MemoryNotesEditor from './MemoryNotesEditor.vue'
import MemoryInjectionPreview from './MemoryInjectionPreview.vue'

// The workspace's agent memories: filter by type, full-text search (Enter),
// open one, delete one or clear the filtered set (both confirmed).

const props = defineProps<{
  workspaceId: string | null
}>()

const emit = defineEmits<{
  (e: 'select-memory', entry: MemoryEntry): void
}>()

const {
  memories,
  loading,
  searchQuery,
  searchResults,
  fetchMemories,
  search,
  deleteMemory,
  selectMemory,
  clearAllMemories,
} = useMemory()
const { confirm } = useConfirm()

const SNIPPET_CHARS = 80
const ALL_TYPES = ''
// Hot is not a memory type: it is the set of facts every run carries.
const HOT_FILTER = 'hot'
const UNUSED_FILTER = 'unused'
const USER_FILTER = 'user_profile'
const UNUSED_NOTE = 'Facts never sent to a model and never found by search. Counting began when usage tracking was added, so older facts appear here until they are first used.'
const FILTERS: ChoiceOption[] = [
  { value: ALL_TYPES, label: 'All' },
  { value: HOT_FILTER, label: 'Hot' },
  { value: 'long_term', label: MEMORY_TYPE_LABEL.long_term },
  { value: 'daily', label: MEMORY_TYPE_LABEL.daily },
  { value: 'session', label: MEMORY_TYPE_LABEL.session },
  { value: USER_FILTER, label: 'User' },
  { value: UNUSED_FILTER, label: 'Unused' },
]

const filterType = ref<string>(ALL_TYPES)
const showAdd = ref(false)
const showPreview = ref(false)
const showNotes = ref(false)
// User facts follow the operator into every workspace, stored under "global".
const listedWorkspace = computed(() => (filterType.value === USER_FILTER ? GLOBAL_MEMORY_WORKSPACE : props.workspaceId))
const isSearching = ref(false)
const displayEntries = computed(() => isSearching.value ? searchResults.value : memories.value)
const filterLabel = computed(() => FILTERS.find((f) => f.value === filterType.value)?.label ?? 'All')

function load() {
  if (!props.workspaceId) return
  if (filterType.value === HOT_FILTER) {
    fetchMemories(props.workspaceId, undefined, 'hot')
  } else if (filterType.value === UNUSED_FILTER) {
    fetchMemories(props.workspaceId, undefined, 'unused')
  } else if (filterType.value === USER_FILTER) {
    fetchMemories(GLOBAL_MEMORY_WORKSPACE, USER_FILTER)
  } else {
    fetchMemories(props.workspaceId, filterType.value || undefined)
  }
}

watch(() => props.workspaceId, (ws) => {
  if (ws) {
    clearSearch()
    load()
  }
}, { immediate: true })

watch(filterType, load)

function handleCreated() {
  showAdd.value = false
  load()
}

// An edit moves updated_at; a fact never touched since it was saved has the same value.
const wasEdited = (entry: MemoryEntry) => !!entry.updated_at && entry.updated_at !== entry.created_at
const isUserWide = (entry: MemoryEntry) => entry.workspace_id === GLOBAL_MEMORY_WORKSPACE
const isHot = (entry: MemoryEntry) => (entry.tags ?? []).includes(HOT_TAG)
// Hot and Unused are views, not buckets of memories: there is nothing sensible to bulk-clear.
const canClear = computed(() => displayEntries.value.length > 0 && filterType.value !== HOT_FILTER && filterType.value !== UNUSED_FILTER)

async function handleSearch() {
  if (!props.workspaceId || !searchQuery.value.trim()) {
    isSearching.value = false
    return
  }
  isSearching.value = true
  await search(props.workspaceId, searchQuery.value)
}

function clearSearch() {
  searchQuery.value = ''
  isSearching.value = false
}

function handleSelect(entry: MemoryEntry) {
  selectMemory(entry)
  emit('select-memory', entry)
}

async function handleClearAll() {
  if (!props.workspaceId) return
  // In All the list also shows user-wide facts, but clearing targets this
  // workspace only: say so, and count only what will really be deleted.
  const clearable = filterType.value === ALL_TYPES ? displayEntries.value.filter((e) => !isUserWide(e)) : displayEntries.value
  if (clearable.length === 0) return
  const keptNote = clearable.length < displayEntries.value.length ? ' Your user-wide facts are kept.' : ''
  const scope = filterLabel.value.toLowerCase()
  const confirmed = await confirm({
    title: `Clear ${filterLabel.value} memories?`,
    message: `Delete all ${clearable.length} ${scope} memories? This cannot be undone.${keptNote}`,
    type: 'error',
    confirmText: `Clear ${filterLabel.value}`,
    cancelText: 'Cancel',
  })
  if (!confirmed) return
  await clearAllMemories(listedWorkspace.value ?? props.workspaceId, filterType.value || undefined)
}

async function handleDelete(entry: MemoryEntry) {
  if (!props.workspaceId) return
  const label = (MEMORY_TYPE_LABEL[entry.memory_type] ?? entry.memory_type).toLowerCase()
  const confirmed = await confirm({
    title: 'Delete memory?',
    message: `Delete this ${label} memory "${entry.title || 'Untitled'}"? This cannot be undone.`,
    type: 'error',
    confirmText: 'Delete',
    cancelText: 'Cancel',
  })
  if (!confirmed) return
  await deleteMemory(entry.workspace_id || props.workspaceId, entry.id)
}

const snippet = (content: string) => (content.length > SNIPPET_CHARS ? `${content.slice(0, SNIPPET_CHARS)}…` : content)
</script>

<template>
  <div class="flex min-h-0 flex-col gap-3">
    <form class="flex flex-wrap items-center gap-2" role="search" @submit.prevent="handleSearch">
      <SearchInput v-model="searchQuery" label="Search memories" class="min-w-[12rem] flex-1" />
      <BaseButton v-if="isSearching" variant="ghost" size="sm" @click="clearSearch">Clear search</BaseButton>
      <BaseButton v-else variant="secondary" size="sm" type="submit">Search</BaseButton>
    </form>

    <div class="flex flex-wrap items-center justify-between gap-2">
      <SegmentedControl v-model="filterType" :options="FILTERS" label="Memory type" />
      <span class="flex flex-wrap items-center gap-2">
        <BaseButton variant="secondary" size="sm" icon="edit" @click="showNotes = !showNotes">Operator notes</BaseButton>
        <BaseButton variant="secondary" size="sm" icon="plus" @click="showAdd = !showAdd">Add memory</BaseButton>
        <BaseButton v-if="canClear" variant="danger" size="sm" icon="trash" @click="handleClearAll">Clear {{ filterLabel.toLowerCase() }}</BaseButton>
      </span>
    </div>

    <MemoryImportExport v-if="workspaceId" :workspace-id="workspaceId" @imported="load" />

    <MemoryNotesEditor v-if="showNotes && workspaceId" :workspace-id="workspaceId" />

    <MemoryAddForm v-if="showAdd && workspaceId" :workspace-id="workspaceId" @created="handleCreated" />

    <p v-if="filterType === UNUSED_FILTER" class="m-0 text-[length:var(--text-small)] text-muted">{{ UNUSED_NOTE }}</p>

    <LoadingState v-if="loading" label="Loading memories" :rows="3" />
    <template v-else>
      <p v-if="isSearching && searchResults.length > 0" class="m-0 font-mono text-[length:var(--text-small)] text-muted">
        {{ searchResults.length }} search result{{ searchResults.length === 1 ? '' : 's' }}
      </p>
      <EmptyState
        v-if="displayEntries.length === 0"
        :title="isSearching ? 'No memories match' : 'No memories yet'"
        body="Memories are saved when the agent uses memory_update during conversations, or add one yourself."
      />
      <ul v-else class="m-0 flex list-none flex-col p-0">
        <li v-for="entry in displayEntries" :key="entry.id" class="flex items-start gap-2 border-b border-hairline py-2 last:border-b-0">
          <button
            type="button"
            data-test="memory-item"
            class="flex min-w-0 flex-1 flex-col items-start gap-1 rounded-[var(--radius-sm)] p-1 text-left hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
            @click="handleSelect(entry)"
          >
            <span class="flex min-w-0 max-w-full items-center gap-2">
              <StatusTag state="neutral" :label="MEMORY_TYPE_LABEL[entry.memory_type] ?? entry.memory_type" />
              <StatusTag v-if="isUserWide(entry)" state="info" label="All workspaces" />
              <StatusTag v-if="isHot(entry)" state="success" label="Always" />
              <StatusTag v-if="isHot(entry) && entry.priority === PRIORITY_HIGH" state="info" label="High priority" />
              <span class="truncate text-primary">{{ entry.title || 'Untitled' }}</span>
            </span>
            <span class="text-[length:var(--text-small)] text-muted">{{ snippet(entry.content) }}</span>
            <span data-test="memory-usage" class="font-mono text-[length:var(--text-micro)] text-faint">{{ formatMemoryUsage(entry) }}</span>
            <span class="font-mono text-[length:var(--text-micro)] text-faint">
              <time :datetime="asUtc(entry.created_at)" :title="formatAbsoluteTime(asUtc(entry.created_at))">Saved {{ formatRelativeTime(asUtc(entry.created_at)) }}</time>
              <template v-if="wasEdited(entry)"> · <time :datetime="asUtc(entry.updated_at)" :title="formatAbsoluteTime(asUtc(entry.updated_at))">Edited {{ formatRelativeTime(asUtc(entry.updated_at)) }}</time></template>
              <template v-if="entry.last_used_at"> · <time :datetime="asUtc(entry.last_used_at)" :title="formatAbsoluteTime(asUtc(entry.last_used_at))">Last used {{ formatRelativeTime(asUtc(entry.last_used_at)) }}</time></template>
            </span>
          </button>
          <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete memory ${entry.title || 'Untitled'}`" @click="handleDelete(entry)" />
        </li>
      </ul>
    </template>

    <div class="flex flex-col gap-2 border-t border-hairline pt-3">
      <BaseButton variant="ghost" size="sm" class="self-start" @click="showPreview = !showPreview">
        {{ showPreview ? 'Hide what the model receives' : 'Show what the model receives' }}
      </BaseButton>
      <MemoryInjectionPreview v-if="showPreview && workspaceId" :workspace-id="workspaceId" />
    </div>
  </div>
</template>
