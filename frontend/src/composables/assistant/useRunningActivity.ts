import { readonly, ref, watch, type Ref } from 'vue'
import { usePolling } from '../ui/usePolling'
import { AssistantService } from '../../services/assistant/assistantService'
import { POLL_INTERVAL_MS } from '../../constants/api'
import type { ActiveRunsResponse } from '../../types/assistant'

// useRunningActivity polls the backend's authoritative per-workspace "active
// runs" endpoint and exposes it as reactive state. It is the source of truth for
// the workspace-scoped "something is running" notifications (assistant glow,
// the correct history row after a refresh) — the backend cannot miss a
// completion the way sticky client-side flags can.
//
// Run-scheduler lane state is NOT read here: the lane snapshot is global, so it
// lives in useGlobalRunActivity and is shared with the header indicator.
//
// The state is a module-level singleton (like useAssistant), so readers such
// as runningActivitySnapshot() share it; the polling itself belongs to the one
// view that calls useRunningActivity and runs only while that view is on screen.
const assistantRunning = ref(false)
const automationRunning = ref(false)
const loading = ref(false)
const error = ref<string | null>(null)
// assistantConversationId is the conversation ID of the assistant run currently
// executing for the workspace ("" when none). Backed by the authoritative
// /active-runs response so the UI can mark the correct history row as running
// after a refresh — the per-session running flag itself is not persisted.
const assistantConversationId = ref('')
// assistantQueued is true while the chat run is registered but not yet admitted
// to the run scheduler, so the UI can show "waiting" instead of a running glow.
const assistantQueued = ref(false)
// The workspace the fields above describe (the last successfully polled one), so
// a consumer never attributes a conversation id to the wrong workspace.
const polledWorkspaceId = ref<string | null>(null)

async function refresh(workspaceId: string | null) {
  if (!workspaceId) return
  loading.value = true
  try {
    const data: ActiveRunsResponse = await AssistantService.getActiveRuns(workspaceId)
    assistantRunning.value = data.assistant_running
    automationRunning.value = data.automation_running
    assistantConversationId.value = data.assistant_conversation_id || ''
    assistantQueued.value = data.assistant_queued ?? false
    polledWorkspaceId.value = workspaceId
    error.value = null
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load active runs'
  } finally {
    loading.value = false
  }
}

/**
 * Read-only view of the last poll, without starting the poller: for consumers
 * that must not bind the single interval to a workspace (run notifications).
 */
export function runningActivitySnapshot() {
  return { assistantConversationId: readonly(assistantConversationId), polledWorkspaceId: readonly(polledWorkspaceId) }
}

/**
 * Polls the viewed workspace's running state while the owning view is on
 * screen (plan D19 / Phase 6 leak audit): the poll pauses while a kept-alive
 * view sits in the background and stops when it unmounts — it used to run for
 * the rest of the session after the first visit. Call it from one view only.
 */
export function useRunningActivity(workspaceId: Readonly<Ref<string | null>>) {
  void refresh(workspaceId.value)
  usePolling(() => void refresh(workspaceId.value), POLL_INTERVAL_MS)

  // Re-poll immediately when the active workspace changes so the indicator
  // always reflects the workspace being viewed.
  watch(workspaceId, (ws) => refresh(ws))

  return { assistantRunning, automationRunning, assistantConversationId, assistantQueued, polledWorkspaceId, loading, error }
}
