<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { useConfirm } from '../../../composables/ui/useConfirm'
import type { MemoryEntry } from '../../../types/memory'
import Icon from '../../icons/Icon.vue'

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

const filterType = ref<string>('')
const isSearching = ref(false)

const displayEntries = computed(() => isSearching.value ? searchResults.value : memories.value)

const filterLabel = computed(() => {
  switch (filterType.value) {
    case 'long_term': return 'Permanent'
    case 'daily': return 'Daily'
    case 'user_profile': return 'User'
    default: return 'All'
  }
})

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
    title: `Clear ${filterLabel.value} Memories`,
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
  const label = typeLabel(entry.memory_type).toLowerCase()
  const confirmed = await confirm({
    title: 'Delete Memory',
    message: `Delete this ${label} memory "${entry.title || 'Untitled'}"? This cannot be undone.`,
    type: 'error',
    confirmText: 'Delete',
    cancelText: 'Cancel',
  })
  if (!confirmed) return
  await deleteMemory(props.workspaceId, entry.id)
}

function typeLabel(t: string): string {
  switch (t) {
    case 'long_term': return 'Permanent'
    case 'daily': return 'Daily'
    case 'session': return 'Session'
    case 'user_profile': return 'User Profile'
    default: return t
  }
}

function badgeLabel(t: string): string {
  switch (t) {
    case 'long_term': return 'LT'
    case 'daily': return 'D'
    case 'session': return 'S'
    case 'user_profile': return 'U'
    default: return '?'
  }
}

function formatTime(ts: string): string {
  if (!ts) return ''
  const d = new Date(ts.endsWith('Z') ? ts : ts + 'Z')
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div class="panel-container">
    <!-- Search -->
    <div class="search-bar">
      <input
        v-model="searchQuery"
        type="text"
        placeholder="Search memories..."
        class="search-input"
        @keyup.enter="handleSearch"
      />
      <button v-if="isSearching" @click="clearSearch" class="btn-clear-search" title="Clear search">×</button>
      <button v-else @click="handleSearch" class="btn-search" title="Search">🔍</button>
    </div>

    <!-- Filter -->
    <div class="filter-bar">
      <button
        :class="{ 'filter-btn--active': filterType === '' }"
        class="filter-btn"
        @click="filterType = ''"
      >All</button>
      <button
        :class="{ 'filter-btn--active': filterType === 'long_term' }"
        class="filter-btn"
        @click="filterType = 'long_term'"
      >Permanent</button>
      <button
        :class="{ 'filter-btn--active': filterType === 'daily' }"
        class="filter-btn"
        @click="filterType = 'daily'"
      >Daily</button>
      <button
        :class="{ 'filter-btn--active': filterType === 'user_profile' }"
        class="filter-btn"
        @click="filterType = 'user_profile'"
      >User</button>
      <button
        v-if="displayEntries.length > 0"
        @click="handleClearAll"
        class="btn-clear-all ml-auto"
        title="Clear memories"
      >Clear</button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading">Loading...</div>

    <!-- Search results header -->
    <div v-if="isSearching && searchResults.length > 0" class="result-header">
      Search results ({{ searchResults.length }})
    </div>

    <!-- Memory list -->
    <div v-if="!loading && (isSearching ? searchResults : memories).length === 0" class="empty-state">
      <div class="empty-text">No memories yet</div>
      <div class="empty-hint">Memories are saved when the agent uses memory_update during conversations.</div>
    </div>

    <div v-if="!loading" class="memory-list">
      <div
        v-for="entry in (isSearching ? searchResults : memories)"
        :key="entry.id"
        class="memory-item group"
        @click="handleSelect(entry)"
      >
        <div class="memory-header">
          <span
            class="memory-type-badge"
            :class="'memory-type-badge--' + entry.memory_type"
            :title="typeLabel(entry.memory_type)"
          >
            {{ badgeLabel(entry.memory_type) }}
          </span>
          <span class="memory-title">{{ entry.title || 'Untitled' }}</span>
          <button
            @click.stop="handleDelete(entry)"
            class="btn-delete"
            title="Delete memory"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
        <div class="memory-snippet">{{ entry.content.slice(0, 80) }}{{ entry.content.length > 80 ? '...' : '' }}</div>
        <div class="memory-footer">
          <span class="memory-time">{{ formatTime(entry.created_at) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.panel-container { @apply flex flex-col h-full overflow-hidden; }
.search-bar { @apply flex items-center gap-1 p-2 border-b border-gray-200 dark:border-gray-700; }
.search-input { @apply flex-1 px-2 py-1 text-sm bg-transparent border border-gray-300 dark:border-gray-600 rounded; }
.filter-bar { @apply flex gap-1 p-2 border-b border-gray-200 dark:border-gray-700; }
.filter-btn { @apply px-2 py-0.5 text-xs rounded; }
.filter-btn--active { @apply bg-blue-500 text-white; }
.filter-btn:not(.filter-btn--active) { @apply bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300; }
.loading { @apply p-4 text-center text-sm text-gray-500; }
.empty-state { @apply flex flex-col items-center justify-center p-6 text-center; }
.empty-text { @apply text-sm text-gray-500 dark:text-gray-400; }
.empty-hint { @apply text-xs text-gray-400 dark:text-gray-500 mt-1; }
.result-header { @apply px-2 py-1 text-xs font-semibold text-gray-500 bg-gray-50 dark:bg-gray-800; }
.memory-list { @apply flex-1 overflow-y-auto; }
.memory-item { @apply px-3 py-2 border-b border-gray-100 dark:border-gray-800 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800; }
.memory-header { @apply flex items-center gap-2 mb-0.5; }
.memory-title { @apply flex-1 text-sm font-medium text-gray-900 dark:text-gray-100 truncate; }
.memory-type-badge { @apply text-[10px] px-1 py-0.5 rounded font-mono shrink-0; }
.memory-type-badge--long_term { @apply bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300; }
.memory-type-badge--daily { @apply bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300; }
.memory-type-badge--session { @apply bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300; }
.memory-type-badge--user_profile { @apply bg-purple-100 dark:bg-purple-900 text-purple-700 dark:text-purple-300; }
.memory-snippet { @apply text-xs text-gray-500 dark:text-gray-400 line-clamp-2; }
.memory-footer { @apply flex items-center mt-1; }
.memory-time { @apply text-[10px] text-gray-400; }
.btn-delete { @apply opacity-0 group-hover:opacity-100 text-gray-400 hover:text-red-500 transition-opacity; }
.btn-clear-all { @apply px-2 py-0.5 text-xs rounded text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30; }
.btn-clear-search, .btn-search { @apply px-1.5 py-0.5 text-xs rounded hover:bg-gray-200 dark:hover:bg-gray-600; }
</style>
