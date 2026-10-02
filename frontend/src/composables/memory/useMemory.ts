import { ref } from 'vue'
import { MemoryService } from '../../services/memory/memoryService'
import type { MemoryEntry, MemoryImportResult, MemoryInjectionPreview, MemoryPriority, MemoryView, NewMemory, NotesScope, OperatorNotes } from '../../types/memory'
import { GLOBAL_MEMORY_WORKSPACE, HOT_TAG } from '../../constants/memory'

const memories = ref<MemoryEntry[]>([])
const searchResults = ref<MemoryEntry[]>([])
const selectedMemory = ref<MemoryEntry | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const searchQuery = ref('')
const injectionPreview = ref<MemoryInjectionPreview | null>(null)
const operatorNotes = ref<OperatorNotes | null>(null)
// Moves after every successful change to what the model would receive from
// memory, so an open preview reloads instead of showing a stale prompt.
const memoryRevision = ref(0)
const markChanged = () => { memoryRevision.value++ }

export function useMemory() {
  const fetchMemories = async (workspaceId: string, type?: string, view?: MemoryView) => {
    loading.value = true
    error.value = null
    try {
      const own = (await MemoryService.list(workspaceId, type, view)) || []
      // The unfiltered view shows everything that applies to the workspace: its own
      // facts plus the user-wide ones, which are sent to the model in every
      // workspace. A failure to load the user-wide ones must not hide the rest.
      const everything = !type && !view && workspaceId !== GLOBAL_MEMORY_WORKSPACE
      const userWide = everything ? await MemoryService.list(GLOBAL_MEMORY_WORKSPACE, undefined, undefined).catch(() => []) : []
      memories.value = [...own, ...(userWide || [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at))
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
      markChanged()
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
      markChanged()
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

  // Resolves true when saved; on failure error carries the reason and the
  // caller keeps what the operator typed.
  const createMemory = async (workspaceId: string, memory: NewMemory): Promise<boolean> => {
    error.value = null
    try {
      await MemoryService.create(workspaceId, memory)
      markChanged()
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to save memory'
      return false
    }
  }

  // Promote or demote a fact without touching its wording. Resolves true on success.
  const setHot = async (entry: MemoryEntry, hot: boolean): Promise<boolean> => {
    error.value = null
    try {
      await MemoryService.update(entry.workspace_id, entry.id, entry.title, entry.content, { hot })
      markChanged()
      const rest = (entry.tags ?? []).filter((t) => t !== HOT_TAG)
      const tags = hot ? [...rest, HOT_TAG] : rest
      memories.value = memories.value.map((m) => (m.id === entry.id && m.workspace_id === entry.workspace_id ? { ...m, tags } : m))
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to update memory'
      return false
    }
  }

  // Set how long a fact survives when the budget cuts the tail. Resolves true on success.
  const setPriority = async (entry: MemoryEntry, priority: MemoryPriority): Promise<boolean> => {
    error.value = null
    try {
      await MemoryService.update(entry.workspace_id, entry.id, entry.title, entry.content, { priority })
      markChanged()
      memories.value = memories.value.map((m) => (m.id === entry.id && m.workspace_id === entry.workspace_id ? { ...m, priority } : m))
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to update memory'
      return false
    }
  }

  const fetchInjectionPreview = async (workspaceId: string, model: string) => {
    error.value = null
    try {
      injectionPreview.value = await MemoryService.injectionPreview(workspaceId, model)
    } catch (err) {
      injectionPreview.value = null
      error.value = err instanceof Error ? err.message : 'Failed to load the memory preview'
    }
  }

  const fetchNotes = async (workspaceId: string) => {
    error.value = null
    try {
      operatorNotes.value = await MemoryService.getNotes(workspaceId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load the operator notes'
    }
  }

  // Resolves true when saved; the stored notes are updated so the editor and
  // any other reader see the new text without another round trip.
  const saveNotes = async (workspaceId: string, scope: NotesScope, content: string): Promise<boolean> => {
    error.value = null
    try {
      await MemoryService.saveNotes(workspaceId, scope, content)
      markChanged()
      if (operatorNotes.value) operatorNotes.value = { ...operatorNotes.value, [scope]: content.trim() }
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to save the operator notes'
      return false
    }
  }

  // Resolves the markdown text, or null with error set.
  const exportMemory = async (workspaceId: string): Promise<string | null> => {
    error.value = null
    try {
      return await MemoryService.exportMarkdown(workspaceId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to export memory'
      return null
    }
  }

  // Resolves what the import did, or null with error set (nothing was added).
  const importMemory = async (workspaceId: string, markdown: string): Promise<MemoryImportResult | null> => {
    error.value = null
    try {
      const result = await MemoryService.importMarkdown(workspaceId, markdown)
      if (result.created > 0) markChanged()
      return result
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to import memory'
      return null
    }
  }

  const clearAllMemories = async (workspaceId: string, type?: string) => {
    loading.value = true
    error.value = null
    try {
      const result = await MemoryService.clearAll(workspaceId, type)
      markChanged()
      memories.value = []
      searchResults.value = []
      selectedMemory.value = null
      return result.deleted
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to clear memories'
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
    injectionPreview,
    operatorNotes,
    memoryRevision,
    fetchMemories,
    search,
    deleteMemory,
    updateMemory,
    createMemory,
    setHot,
    setPriority,
    fetchInjectionPreview,
    fetchNotes,
    saveNotes,
    exportMemory,
    importMemory,
    clearAllMemories,
    selectMemory,
  }
}
