import { ref, watch } from 'vue'
import { AssistantService } from '../services/assistantService'
import { useAssistantSSE } from './useAssistantSSE'
import { useMessageBuilder } from '../utils/messageBuilder'
import type { AssistantMessage, SessionBrief } from '../types/assistant'

const loading = ref(false)
const error = ref<string | null>(null)
const currentSessionId = ref<string | null>(null)
const messages = ref<AssistantMessage[]>([])
const sessions = ref<SessionBrief[]>([])
const activeWorkspaceId = ref<string | null>(null)
const abortController = ref<AbortController | null>(null)

export function useAssistant() {
  const builder = useMessageBuilder(messages)

  const sse = useAssistantSSE(
    () => activeWorkspaceId.value || '',
    (ev) => builder.handleEvent(ev),
  )

  const streamingContent = sse.streamingContent
  const liveEvents = sse.liveEvents
  const pendingDecision = sse.pendingDecision
  const sseConnected = sse.isConnected
  const clearLiveEvents = sse.reset
  const streaming = builder.streaming
  const thinking = builder.thinking
  const liveReasoning = builder.liveReasoning
  const paused = builder.paused

  const cancel = () => {
    if (abortController.value) {
      abortController.value.abort()
      abortController.value = null
    }
    sse.disconnect()
    builder.reset()
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
      if (!session) {
        error.value = 'Session not found'
        newSession()
        return
      }
      currentSessionId.value = session.id
      messages.value = session.history || []
    } catch (err) {
      console.error('Failed to load session, starting fresh:', err)
      newSession()
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

    // For new conversations (no active session), clear stale messages
    // so previous session data doesn't leak into the new chat
    if (!currentSessionId.value) {
      messages.value = []
    }

    messages.value.push({ role: 'user', content: text })

    loading.value = true
    error.value = null

    sse.reset()
    builder.reset()
    sse.connect()
    builder.resetPauseTimer()

    // Wait for SSE connection before sending the agent request so
    // tool_call/tool_result events arrive live instead of all at
    // once from the recent buffer after the agent finishes
    if (!sseConnected.value) {
      await new Promise<void>((resolve) => {
        const stop = watch(sseConnected, (connected) => {
          if (connected) {
            stop()
            resolve()
          }
        })
        setTimeout(() => { stop(); resolve() }, 5000)
      })
    }

    abortController.value = new AbortController()

    try {
      const response = await AssistantService.sendMessage({
        workspace_id: workspaceId,
        conversation_id: currentSessionId.value || undefined,
        message: text,
      }, abortController.value.signal)

      sse.disconnect()

      if (!currentSessionId.value && response.conversation_id) {
        currentSessionId.value = response.conversation_id
        sessions.value.unshift({
          id: response.conversation_id,
          snippet: text.substring(0, 80),
          updated_at: new Date().toISOString(),
        })
      }

      builder.finalize(response.reply)
      builder.reset()

      sse.reset()

      await fetchSessions(workspaceId)

    } catch (err) {
      if ((err as any)?.name === 'AbortError') {
        error.value = null
      } else {
        error.value = err instanceof Error ? err.message : 'Failed to send message'
        console.error(err)
      }
      sse.disconnect()
      builder.reset()
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
      } else if (!currentSessionId.value && messages.value.length > 0) {
        messages.value = []
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
    streaming,
    thinking,
    liveReasoning,
    paused,
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
