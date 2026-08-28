import { ref, watch, type Ref } from 'vue'
import { AssistantService } from '../../services/assistant/assistantService'
import { POLL_INTERVAL_MS } from '../../constants/api'
import type { ActiveRunsResponse } from '../../types/assistant'

// useRunningActivity polls the backend's authoritative per-workspace "active
// runs" endpoint and exposes it as reactive state. It is the single source of
// truth for "something is running" notifications (assistant glow, automation
// indicators, future surfaces) — the backend cannot miss a completion the way
// sticky client-side flags can.
//
// Implemented as a module-level singleton (like useAssistant) so the polling
// interval is shared and lives for the app's lifetime.
const assistantRunning = ref(false)
const automationRunning = ref(false)
const loading = ref(false)
const error = ref<string | null>(null)
// assistantConversationId is the conversation ID of the assistant run currently
// executing for the workspace ("" when none). Backed by the authoritative
// /active-runs response so the UI can mark the correct history row as running
// after a refresh — the per-session running flag itself is not persisted.
const assistantConversationId = ref('')
let timer: ReturnType<typeof setInterval> | null = null

async function refresh(workspaceId: string | null) {
  if (!workspaceId) return
  loading.value = true
  try {
    const data: ActiveRunsResponse = await AssistantService.getActiveRuns(workspaceId)
    assistantRunning.value = data.assistant_running
    automationRunning.value = data.automation_running
    assistantConversationId.value = data.assistant_conversation_id || ''
    error.value = null
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load active runs'
  } finally {
    loading.value = false
  }
}

export function useRunningActivity(workspaceId: Ref<string | null>) {
  function start() {
    if (timer) return
    refresh(workspaceId.value)
    timer = setInterval(() => refresh(workspaceId.value), POLL_INTERVAL_MS)
  }

  // Re-poll immediately when the active workspace changes so the indicator
  // always reflects the workspace being viewed.
  watch(workspaceId, (ws) => refresh(ws))

  start()

  return { assistantRunning, automationRunning, assistantConversationId, loading, error }
}
