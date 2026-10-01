<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { useConfirm } from '../../../composables/ui/useConfirm'
import { asUtc, formatAbsoluteTime, formatRelativeTime } from '../../../utils/format/time'
import { MEMORY_TYPE_LABEL } from '../../../constants/memory'
import type { MemoryEntry } from '../../../types/memory'
import type { ChoiceOption } from '../../../types/ui'
import SegmentedControl from '../../common/forms/SegmentedControl.vue'
import SearchInput from '../../common/forms/SearchInput.vue'
import StatusTag from '../../common/display/StatusTag.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import LoadingState from '../../common/feedback/LoadingState.vue'
import EmptyState from '../../common/feedback/EmptyState.vue'

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
const FILTERS: ChoiceOption[] = [
  { value: ALL_TYPES, label: 'All' },
  { value: 'long_term', label: MEMORY_TYPE_LABEL.long_term },
  { value: 'daily', label: MEMORY_TYPE_LABEL.daily },
  { value: 'user_profile', label: 'User' },
]

const filterType = ref<string>(ALL_TYPES)
const isSearching = ref(false)
const displayEntries = computed(() => isSearching.value ? searchResults.value : memories.value)
const filterLabel = computed(() => FILTERS.find((f) => f.value === filterType.value)?.label ?? 'All')

watch(() => props.workspaceId, (ws) => {
  if (ws) {
    clearSearch()
    fetchMemories(ws, filterType.value || undefined)
  }
}, { immediate: true })

watch(filterType, () => {
  if (props.workspaceId) {
    fetchMemories(props.workspaceId, filterType.value || undefined)
  }
})

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
  if (!props.workspaceId || displayEntries.value.length === 0) return
  const scope = filterLabel.value.toLowerCase()
  const confirmed = await confirm({
    title: `Clear ${filterLabel.value} memories?`,
    message: `Delete all ${displayEntries.value.length} ${scope} memories? This cannot be undone.`,
    type: 'error',
    confirmText: `Clear ${filterLabel.value}`,
    cancelText: 'Cancel',
  })
  if (!confirmed) return
  await clearAllMemories(props.workspaceId, filterType.value || undefined)
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
  await deleteMemory(props.workspaceId, entry.id)
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
      <BaseButton v-if="displayEntries.length > 0" variant="danger" size="sm" icon="trash" @click="handleClearAll">Clear {{ filterLabel.toLowerCase() }}</BaseButton>
    </div>

    <LoadingState v-if="loading" label="Loading memories" :rows="3" />
    <template v-else>
      <p v-if="isSearching && searchResults.length > 0" class="m-0 font-mono text-[length:var(--text-small)] text-muted">
        {{ searchResults.length }} search result{{ searchResults.length === 1 ? '' : 's' }}
      </p>
      <EmptyState
        v-if="displayEntries.length === 0"
        :title="isSearching ? 'No memories match' : 'No memories yet'"
        body="Memories are saved when the agent uses memory_update during conversations."
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
              <span class="truncate text-primary">{{ entry.title || 'Untitled' }}</span>
            </span>
            <span class="text-[length:var(--text-small)] text-muted">{{ snippet(entry.content) }}</span>
            <time
              :datetime="asUtc(entry.created_at)"
              :title="formatAbsoluteTime(asUtc(entry.created_at))"
              class="font-mono text-[length:var(--text-micro)] text-faint"
            >{{ formatRelativeTime(asUtc(entry.created_at)) }}</time>
          </button>
          <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete memory ${entry.title || 'Untitled'}`" @click="handleDelete(entry)" />
        </li>
      </ul>
    </template>
  </div>
</template>
