import { ref } from 'vue'
import { AssistantService } from '../services/assistantService'
import type { SessionBrief } from '../types/assistant'

const sessions = ref<SessionBrief[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

export function useConversations() {
  const fetchSessions = async (workspaceId: string) => {
    loading.value = true
    error.value = null
    try {
      const result = await AssistantService.listSessions(workspaceId)
      sessions.value = result || []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to fetch sessions'
    } finally {
      loading.value = false
    }
  }

  const renameSession = async (workspaceId: string, sessionId: string, title: string) => {
    try {
      await AssistantService.renameSession(workspaceId, sessionId, title)
      const s = sessions.value.find(s => s.id === sessionId)
      if (s) s.snippet = title
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to rename session'
    }
  }

  const deleteSession = async (workspaceId: string, sessionId: string) => {
    try {
      await AssistantService.deleteSession(workspaceId, sessionId)
      sessions.value = sessions.value.filter(s => s.id !== sessionId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete session'
    }
  }

  return {
    sessions,
    loading,
    error,
    fetchSessions,
    renameSession,
    deleteSession,
  }
}
