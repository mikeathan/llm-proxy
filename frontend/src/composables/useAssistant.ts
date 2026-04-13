import { ref } from 'vue'
import { AssistantService } from '../services/assistantService'
import type { AssistantMessage, SessionBrief, AssistantSession } from '../types/assistant'

const loading = ref(false)
const error = ref<string | null>(null)
const currentSessionId = ref<string | null>(null)
const messages = ref<AssistantMessage[]>([])
const sessions = ref<SessionBrief[]>([])
const activeWorkspaceId = ref<string | null>(null)

export function useAssistant() {

  const fetchSessions = async (workspaceId: string) => {
    loading.value = true
    error.value = null
    try {
      sessions.value = await AssistantService.listSessions(workspaceId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to fetch sessions'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const loadSession = async (workspaceId: string, sessionId: string) => {
    loading.value = true
    error.value = null
    try {
      const session = await AssistantService.getSession(workspaceId, sessionId)
      currentSessionId.value = session.id
      messages.value = session.history || []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load session'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const newSession = () => {
    currentSessionId.value = null
    messages.value = []
  }

  const sendMessage = async (workspaceId: string, text: string) => {
    if (!text.trim()) return

    // Optimistically add user message
    messages.value.push({
      role: 'user',
      content: text,
    })

    loading.value = true
    error.value = null

    try {
      const response = await AssistantService.sendMessage({
        workspace_id: workspaceId,
        conversation_id: currentSessionId.value || undefined,
        message: text,
      })

      // Update session ID if it was a new session
      if (!currentSessionId.value && response.conversation_id) {
        currentSessionId.value = response.conversation_id
      }

      // Add assistant response
      messages.value.push({
        role: 'assistant',
        content: response.reply,
      })

      // Refresh session list so the new snippet appears
      await fetchSessions(workspaceId)

    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to send message'
      console.error(err)
      // On error, let's keep the user message so they can see what failed, 
      // but maybe add a system message or toast (handled by UI).
    } finally {
      loading.value = false
    }
  }

  const deleteSession = async (workspaceId: string, sessionId: string) => {
    loading.value = true
    error.value = null
    try {
      await AssistantService.deleteSession(workspaceId, sessionId)
      sessions.value = sessions.value.filter(s => s.id !== sessionId)
      if (currentSessionId.value === sessionId) {
        newSession()
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete session'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  return {
    loading,
    error,
    currentSessionId,
    messages,
    sessions,
    fetchSessions,
    loadSession,
    newSession,
    sendMessage,
    deleteSession,
    activeWorkspaceId,
  }
}
