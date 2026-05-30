<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMemory } from '../../../composables/useMemory'
import type { MemoryEntry } from '../../../types/memory'

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
} = useMemory()

const filterType = ref<string>('')
const isSearching = ref(false)

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
          <span class="memory-title">{{ entry.title || 'Untitled' }}</span>
          <span class="memory-type-badge" :class="'memory-type-badge--' + entry.memory_type">
            {{ entry.memory_type === 'long_term' ? 'LT' : entry.memory_type === 'daily' ? 'D' : entry.memory_type === 'session' ? 'S' : 'U' }}
          </span>
        </div>
        <div class="memory-snippet">{{ entry.content.slice(0, 80) }}{{ entry.content.length > 80 ? '...' : '' }}</div>
        <div class="memory-footer">
          <span class="memory-time">{{ formatTime(entry.created_at) }}</span>
          <button
            @click.stop="deleteMemory(props.workspaceId!, entry.id)"
            class="btn-delete opacity-0 group-hover:opacity-100"
            title="Delete memory"
          >×</button>
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
.memory-header { @apply flex items-center justify-between mb-0.5; }
.memory-title { @apply text-sm font-medium text-gray-900 dark:text-gray-100 truncate; }
.memory-type-badge { @apply text-[10px] px-1 py-0.5 rounded font-mono; }
.memory-type-badge--long_term { @apply bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300; }
.memory-type-badge--daily { @apply bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300; }
.memory-type-badge--session { @apply bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300; }
.memory-type-badge--user_profile { @apply bg-purple-100 dark:bg-purple-900 text-purple-700 dark:text-purple-300; }
.memory-snippet { @apply text-xs text-gray-500 dark:text-gray-400 line-clamp-2; }
.memory-footer { @apply flex items-center justify-between mt-1; }
.memory-time { @apply text-[10px] text-gray-400; }
.btn-delete { @apply text-gray-400 hover:text-red-500 text-sm transition-opacity; }
.btn-clear-search, .btn-search { @apply px-1.5 py-0.5 text-xs rounded hover:bg-gray-200 dark:hover:bg-gray-600; }
</style>
