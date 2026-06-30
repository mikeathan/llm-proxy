import { ref, watch } from 'vue'
import { AssistantService } from '../../services/assistantService'
import { useAssistantSSE } from './useAssistantSSE'
import { useMessageBuilder } from '../../utils/message/messageBuilder'
import { buildSegmentsFromHistory } from '../../utils/message/turnGrouper'
import type { SessionLifecyclePayload } from './useAssistantSSE'
import type { AssistantMessage, SessionBrief } from '../../types/assistant'

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
    applySessionUpdate,
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

  const cancel = async () => {
    const ws = activeWorkspaceId.value
    if (!ws) return
    // Cancel by workspace — conversation_id is optional.  When the user
    // stops the first send before the response returns a session_id, we
    // still need the cancel signal to reach the backend.  The cancel
    // response may carry back the real session_id; we use it to keep the
    // cancelled turn in the same conversation as the next send.
    try {
      const resp = await AssistantService.cancelAgent(ws, currentSessionId.value ?? '')
      if (resp.conversation_id && !currentSessionId.value) {
        currentSessionId.value = resp.conversation_id
        const lastUser = [...messages.value].reverse().find(m => m.role === 'user')
        sessions.value.unshift({
          id: resp.conversation_id,
          snippet: (lastUser?.content ?? '').substring(0, 80),
          updated_at: new Date().toISOString(),
        })
      }
    } catch (err) {
      console.warn('cancel agent request failed', err)
    }
    // Do NOT abort the original HTTP request.  The cancel signal causes
    // the in-flight `await AssistantService.sendMessage(...)` to resolve
    // with the cancel response, which contains `conversation_id` and is
    // the only place the frontend learns the session id when the first
    // send is cancelled.  Aborting would discard it.
    sse.disconnect()
    builder.reset()
    loading.value = false
  }

  const fetchSessions = async (workspaceId: string) => {
    loading.value = true
    error.value = null
    try {
      // Preserve in-memory running state — the disk doesn't store it and
      // SSE lifecycle events update it faster than ListSessions re-reads.
      const runningIds = new Set(sessions.value.filter(s => s.running).map(s => s.id))
      const result = await AssistantService.listSessions(workspaceId)
      sessions.value = (result || []).map(s => ({
        ...s,
        running: s.running ?? runningIds.has(s.id)
      }))
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to fetch sessions'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const loadSession = async (workspaceId: string, sessionId: string) => {
    // Running session: read user input from disk so the prompt is visible
    // immediately, then reconnect SSE to replay live agent events.
    if (sessions.value.find(s => s.id === sessionId)?.running) {
      currentSessionId.value = sessionId
      const session = await AssistantService.getSession(workspaceId, sessionId)
      if (session) {
        messages.value = buildSegmentsFromHistory(session.history || [])
      }
      sse.reset()
      sse.connect()
      return
    }

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
      messages.value = buildSegmentsFromHistory(session.history || [])
      // Apply every cancelled turn's marker so all of them are shown as
      // "Response interrupted" in the UI.  Older sessions may still have
      // the legacy metadata keys; honour them as a fallback.
      const indices = session.cancelled_indices
      if (Array.isArray(indices)) {
        for (const idx of indices) {
          if (typeof idx === 'number' && messages.value[idx]) {
            messages.value[idx].canceled = true
          }
        }
      }
      const legacyMsgIdx = session.metadata?.canceled_message_index
      if (typeof legacyMsgIdx === 'number' && messages.value[legacyMsgIdx]) {
        messages.value[legacyMsgIdx].canceled = true
      }
      const legacyUserIdx = session.metadata?.canceled_user_message_index
      if (typeof legacyUserIdx === 'number' && messages.value[legacyUserIdx]) {
        messages.value[legacyUserIdx].canceled = true
      }
      if (messages.value.length === 0) {
        console.warn('Loaded session has empty history', sessionId)
      }
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

      if (response.canceled) {
        // Mark the current turn's messages by scanning from the end for the
        // last user message (the one we just sent) and any assistant messages
        // after it.  This avoids relying on backend indices that are offset
        // by the system prompt not present in messages.value during the live
        // state.
        const msgs = messages.value
        for (let i = msgs.length - 1; i >= 0; i--) {
          const m = msgs[i]
          if (!m) continue
          if (m.role === 'user') {
            m.canceled = true
            for (let j = i + 1; j < msgs.length; j++) {
              const a = msgs[j]
              if (a && a.role === 'assistant') a.canceled = true
            }
            break
          }
        }
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

  function applySessionUpdate(p: SessionLifecyclePayload) {
    const cid = p.conversation_id
    if (!cid) return
    const idx = sessions.value.findIndex(s => s.id === cid)
    if (p.phase === "session_started") {
      if (idx === -1) {
        sessions.value.unshift({
          id: cid,
          snippet: p.snippet ?? "",
          updated_at: new Date().toISOString(),
          running: true,
        })
      } else {
        const existing = sessions.value[idx]
        if (existing) sessions.value[idx] = { ...existing, running: true, snippet: p.snippet ?? existing.snippet }
      }
    } else if (p.phase === "session_progress") {
      if (idx !== -1) {
        const existing = sessions.value[idx]
        if (existing) sessions.value[idx] = { ...existing, snippet: p.snippet ?? existing.snippet }
      }
    } else if (p.phase === "session_completed") {
      if (idx !== -1) {
        const existing = sessions.value[idx]
        if (existing) sessions.value[idx] = { ...existing, running: false }
      }
    }
  }

  const cancelSession = async (workspaceId: string, sessionId: string) => {
    try {
      await AssistantService.cancelAgent(workspaceId, sessionId)
    } catch (err) {
      console.warn('cancel session failed', err)
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
    cancelSession,
    activeWorkspaceId,
  }
}
