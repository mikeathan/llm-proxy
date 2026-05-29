import { ref } from 'vue'
import { MemoryService } from '../services/memoryService'
import type { MemoryEntry } from '../types/memory'

const memories = ref<MemoryEntry[]>([])
const searchResults = ref<MemoryEntry[]>([])
const selectedMemory = ref<MemoryEntry | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const searchQuery = ref('')

export function useMemory() {
  const fetchMemories = async (workspaceId: string, type?: string) => {
    loading.value = true
    error.value = null
    try {
      const result = await MemoryService.list(workspaceId, type)
      memories.value = result || []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to fetch memories'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const search = async (workspaceId: string, query: string) => {
    if (!query.trim()) {
      searchResults.value = []
      return
    }
    loading.value = true
    error.value = null
    try {
      const result = await MemoryService.search(workspaceId, query)
      searchResults.value = result || []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to search memories'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const deleteMemory = async (workspaceId: string, id: number) => {
    loading.value = true
    error.value = null
    try {
      await MemoryService.delete(workspaceId, id)
      memories.value = memories.value.filter(m => m.id !== id)
      if (selectedMemory.value?.id === id) {
        selectedMemory.value = null
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete memory'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const updateMemory = async (workspaceId: string, id: number, title: string, content: string) => {
    loading.value = true
    error.value = null
    try {
      await MemoryService.update(workspaceId, id, title, content)
      if (selectedMemory.value?.id === id) {
        selectedMemory.value = { ...selectedMemory.value, title, content }
      }
      await fetchMemories(workspaceId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to update memory'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const selectMemory = (entry: MemoryEntry | null) => {
    selectedMemory.value = entry
  }

  return {
    memories,
    searchResults,
    selectedMemory,
    loading,
    error,
    searchQuery,
    fetchMemories,
    search,
    deleteMemory,
    updateMemory,
    selectMemory,
  }
}
