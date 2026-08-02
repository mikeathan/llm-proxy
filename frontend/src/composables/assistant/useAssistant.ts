import { ref, computed, watch } from 'vue'
import { AssistantService } from '../../services/assistant/assistantService'
import { useAssistantSSE } from './useAssistantSSE'
import { useMessageBuilder } from '../../utils/message/messageBuilder'
import { buildSegmentsFromHistory } from '../../utils/message/turnGrouper'
import type { SessionLifecyclePayload } from './useAssistantSSE'
import type { AssistantMessage, SessionBrief } from '../../types/assistant'
import { clearRunningFlags } from '../../utils/assistant/running'

const loading = ref(false)
const error = ref<string | null>(null)
const currentSessionId = ref<string | null>(null)
const messages = ref<AssistantMessage[]>([])
const sessions = ref<SessionBrief[]>([])
const activeWorkspaceId = ref<string | null>(null)
  const abortController = ref<AbortController | null>(null)

const runningSessions = computed(() => sessions.value.filter((s) => s.running))

// reconcileRunning heals sticky `running` flags. When the authoritative
// backend reports nothing is executing for the workspace, any locally-flagged
// running session is cleared so the "running" indicator cannot get stuck on
// after a missed completion event.
function reconcileRunning(assistantRunning: boolean) {
  if (!assistantRunning && sessions.value.some((s) => s.running)) {
    sessions.value = clearRunningFlags(sessions.value)
  }
}

export function useAssistant() {
  // finalizeOn:'lifecycle' mirrors automation: the builder finalizes from the
  // SSE lifecycle{completed} event (which carries the full answer), so chat and
  // automation share one completion path and the answer is never lost.
  const builder = useMessageBuilder(messages, { finalizeOn: 'lifecycle' })

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
  const connectSSE = () => sse.connect()
  const streaming = builder.streaming
  const thinking = builder.thinking
  const liveReasoning = builder.liveReasoning
  const paused = builder.paused
  const phase = builder.phase

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
      // Capture after the API call so SSE events that arrived during the
      // request are reflected (avoids a race between connectSSE and fetch).
      const result = await AssistantService.listSessions(workspaceId)
      const runningIds = new Set(sessions.value.filter(s => s.running).map(s => s.id))
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
    builder.reset()
    // Running session: read user input from disk for instant display,
    // then let the existing SSE connection stream live events into the
    // chat.  Don't reconnect — the workspace-level SSE is already
    // connected and dropping it would lose events published in the gap.
    if (sessions.value.find(s => s.id === sessionId)?.running) {
      loading.value = true
      currentSessionId.value = sessionId
      const session = await AssistantService.getSession(workspaceId, sessionId)
      if (session) {
        messages.value = buildSegmentsFromHistory(session.history || [])
        builder.reset()
      }
      sse.reset()
      return
    }

    loading.value = true
    error.value = null
    sse.reset()
    try {
      const session = await AssistantService.getSession(workspaceId, sessionId)
      if (!session) {
        error.value = 'Session not found'
        newSession()
        return
      }
      currentSessionId.value = session.id
      messages.value = buildSegmentsFromHistory(session.history || [])
      // Drive the (now historical) turn's bubble into its done state so the
      // result and the expandable reasoning/tool-call inset render — the same
      // as a finished live run. builder.reset() above left phase='idle', which
      // would hide the already-loaded answer. The running-session branch
      // returns before here, so live SSE still owns phase for in-flight runs.
      phase.value = 'done'
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

    let aborted = false
    try {
      // The POST no longer returns the finished answer — the backend starts a
      // detached background run and responds 202 {status:"running"} immediately.
      // The run survives page refreshes / client disconnects; it is observed
      // live over the SSE event bus. We do NOT pass an AbortSignal: aborting
      // the fetch must not cancel the run (the cancel button does that via
      // /assistant/cancel). We also must NOT disconnect SSE or finalize here —
      // the SSE lifecycle{completed} event drives finalization, and keeping SSE
      // connected lets a refresh reconnect and resume streaming.
      const response = await AssistantService.sendMessage({
        workspace_id: workspaceId,
        conversation_id: currentSessionId.value || undefined,
        message: text,
      })

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

      // The builder finalizes from the SSE lifecycle{completed} event
      // (finalizeOn:'lifecycle', Hermes-aligned) — the same path automation
      // uses, so the answer can never be lost. Do NOT disconnect SSE or reset
      // the builder here: the run is still live and streaming. Keep loading=true
      // so the UI reflects the in-flight run.

    } catch (err) {
      if ((err as any)?.name === 'AbortError') {
        aborted = true
        error.value = null
      } else {
        error.value = err instanceof Error ? err.message : 'Failed to send message'
        console.error(err)
      }
      sse.disconnect()
      builder.reset()
    } finally {
      if (!aborted) {
        // A navigation abort leaves the run alive; keep loading=true and SSE
        // connected so streaming continues / a refresh can reconnect.
        loading.value = false
      }
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
    await deleteSessionsByIds(workspaceId, sessions.value.map((s) => s.id))
  }

  // deleteSessionsByIds removes a specific set of sessions and resyncs the
  // list once, rather than mutating per call.
  const deleteSessionsByIds = async (workspaceId: string, ids: string[]) => {
    if (ids.length === 0) return
    loading.value = true
    error.value = null
    try {
      for (const id of ids) {
        await AssistantService.deleteSession(workspaceId, id)
      }
      const removed = new Set(ids)
      sessions.value = sessions.value.filter((s) => !removed.has(s.id))
      if (currentSessionId.value && removed.has(currentSessionId.value)) {
        newSession()
      }
      await fetchSessions(workspaceId)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete sessions'
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  function applySessionUpdate(p: SessionLifecyclePayload) {
    if (p.workspace_id && p.workspace_id !== activeWorkspaceId.value) return
    const cid = p.conversation_id
    if (!cid) return
    const idx = sessions.value.findIndex(s => s.id === cid)
    if (p.phase === "session_started") {
      if (currentSessionId.value === null) {
        currentSessionId.value = cid
      }
      // Webhook-triggered sessions bypass sendMessage(), so the user
      // message never enters messages.value.  Push it here so groupTurns()
      // can create the turn.  The loading guard prevents this during
      // manual sends where sendMessage() already pushed it.
      if (p.snippet && !loading.value) {
        currentSessionId.value = cid
        builder.reset()
        builder.resetPauseTimer()
        loading.value = true
        messages.value = []
        messages.value.push({ role: 'user', content: p.snippet })
      }
		if (idx === -1) {
			sessions.value.unshift({
				id: cid,
				snippet: p.snippet ?? "",
				updated_at: new Date().toISOString(),
				running: true,
				source: p.source,
			})
		} else {
			const existing = sessions.value[idx]
			if (existing) sessions.value[idx] = { ...existing, running: true, snippet: p.snippet ?? existing.snippet, source: p.source ?? existing.source }
		}
		} else if (p.phase === "session_progress") {
			if (idx !== -1) {
				const existing = sessions.value[idx]
				// Keep the stable title set on session_started. Progress events
				// carry transient step text ("Step N: ..."), not the title.
				if (existing) sessions.value[idx] = { ...existing, snippet: existing.snippet }
			}
		}
		else if (p.phase === "session_completed") {
			if (idx !== -1) {
				const existing = sessions.value[idx]
				if (existing) sessions.value[idx] = { ...existing, running: false }
			}
			if (cid === currentSessionId.value) {
				loading.value = false
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
    connectSSE,
    streaming,
    thinking,
    liveReasoning,
    paused,
    phase,
    cancel,
    abortController,
    fetchSessions,
    loadSession,
    newSession,
    sendMessage,
    deleteSession,
    deleteAllSessions,
    deleteSessionsByIds,
    cancelSession,
    activeWorkspaceId,
    runningSessions,
    reconcileRunning,
  }
}
