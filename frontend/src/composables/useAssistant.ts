import { ref } from 'vue'
import { AssistantService } from '../services/assistantService'
import { useAssistantSSE } from './useAssistantSSE'
import { toolCallEventToMessage, toolResultEventToMessage } from '../utils/dispatcher'
import type { AssistantMessage, SessionBrief } from '../types/assistant'

const loading = ref(false)
const error = ref<string | null>(null)
const currentSessionId = ref<string | null>(null)
const messages = ref<AssistantMessage[]>([])
const sessions = ref<SessionBrief[]>([])
const activeWorkspaceId = ref<string | null>(null)
const abortController = ref<AbortController | null>(null)

export function useAssistant() {

  const sse = useAssistantSSE(() => activeWorkspaceId.value || '')

  const streamingContent = sse.streamingContent
  const liveEvents = sse.liveEvents
  const pendingDecision = sse.pendingDecision
  const sseConnected = sse.isConnected
  const clearLiveEvents = sse.reset

  const cancel = () => {
    if (abortController.value) {
      abortController.value.abort()
      abortController.value = null
    }
    sse.disconnect()
    loading.value = false
  }

  const fetchSessions = async (workspaceId: string) => {
    loading.value = true
    error.value = null
    try {
      const result = await AssistantService.listSessions(workspaceId)
      sessions.value = result || []
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
    sse.reset()
  }

  const sendMessage = async (workspaceId: string, text: string) => {
    if (!text.trim()) return

    if (activeWorkspaceId.value !== workspaceId) {
      activeWorkspaceId.value = workspaceId
    }

    // Optimistically add user message
    messages.value.push({
      role: 'user',
      content: text,
    })

    loading.value = true
    error.value = null

    // Start SSE for real-time events during execution
    sse.reset()
    sse.connect()

    // Create abort controller for cancellation
    abortController.value = new AbortController()

    try {
      const response = await AssistantService.sendMessage({
        workspace_id: workspaceId,
        conversation_id: currentSessionId.value || undefined,
        message: text,
      }, abortController.value.signal)

      // Update session ID if it was a new session
      if (!currentSessionId.value && response.conversation_id) {
        currentSessionId.value = response.conversation_id
      }

      // Stop SSE once the HTTP response arrives
      sse.disconnect()

      // Clear stale SSE events (message events are carried by reply, tool_call/tool_result come via response.events).
      sse.reset()

      // Handle any tool_call/tool_result events from the response first,
      // so they appear above the final reply in the chat history.
      if (response.events) {
        for (const ev of response.events) {
          if (ev.type === 'tool_call') {
            messages.value.push(toolCallEventToMessage(ev))
          } else if (ev.type === 'tool_result') {
            messages.value.push(toolResultEventToMessage(ev))
          }
        }
      }

      // Add assistant response last — use final reply from HTTP response.
      messages.value.push({
        role: 'assistant',
        content: response.reply,
      })

      // Refresh session list so the new snippet appears
      await fetchSessions(workspaceId)

    } catch (err) {
      if ((err as any)?.name === 'AbortError') {
        // User cancelled — no error to show
        error.value = null
      } else {
        error.value = err instanceof Error ? err.message : 'Failed to send message'
        console.error(err)
      }
      sse.disconnect()
    } finally {
      loading.value = false
      abortController.value = null
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

  const deleteAllSessions = async (workspaceId: string) => {
    const ids = [...sessions.value]
    if (ids.length === 0) return
    loading.value = true
    error.value = null
    try {
      for (const s of ids) {
        await AssistantService.deleteSession(workspaceId, s.id)
      }
      sessions.value = []
      newSession()
      await fetchSessions(workspaceId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete all sessions'
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
    streamingContent,
    liveEvents,
    pendingDecision,
    sseConnected,
    clearLiveEvents,
    cancel,
    abortController,
    fetchSessions,
    loadSession,
    newSession,
    sendMessage,
    deleteSession,
    deleteAllSessions,
    activeWorkspaceId,
  }
}
