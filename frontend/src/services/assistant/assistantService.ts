import type {
  ChatRequestPayload,
  ChatResponsePayload,
  CancelAgentResponse,
  SessionBrief,
  AssistantSession,
  ActiveRunsResponse,
  GlobalActiveRunsResponse,
} from '../../types/assistant'
import { API_ENDPOINTS } from '../../constants/api'

export class AssistantService {
  static async sendMessage(payload: ChatRequestPayload, signal?: AbortSignal): Promise<ChatResponsePayload> {
    const res = await fetch('/admin/api/conversation/message', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      signal,
      body: JSON.stringify({
        workspace_id: payload.workspace_id,
        conversation_id: payload.conversation_id || '',
        message: payload.message,
        timezone: payload.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone,
      }),
    })

    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Chat failed: ${res.status} - ${text}`)
    }

    return res.json()
  }

  static async listSessions(workspaceId: string): Promise<SessionBrief[]> {
    const res = await fetch(`/admin/api/conversation/sessions/${workspaceId}`)
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to list sessions: ${res.status} - ${text}`)
    }
    return res.json()
  }

  static async getSession(workspaceId: string, sessionId: string): Promise<AssistantSession> {
    const res = await fetch(`/admin/api/conversation/sessions/${workspaceId}/${sessionId}`)
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to load session: ${res.status} - ${text}`)
    }
    return res.json()
  }

  static async deleteSession(workspaceId: string, sessionId: string): Promise<void> {
    const res = await fetch(`/admin/api/conversation/sessions/${workspaceId}/${sessionId}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to delete session: ${res.status} - ${text}`)
    }
  }

  static async deleteAllSessions(workspaceId: string): Promise<void> {
    const res = await fetch(`/admin/api/conversation/sessions/${workspaceId}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to delete all sessions: ${res.status} - ${text}`)
    }
  }

  static async renameSession(workspaceId: string, sessionId: string, title: string): Promise<AssistantSession> {
    const res = await fetch(`/admin/api/conversation/sessions/${workspaceId}/${sessionId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to rename session: ${res.status} - ${text}`)
    }
    return res.json()
  }

  static async cancelAgent(
    workspaceId: string,
    sessionId: string
  ): Promise<CancelAgentResponse> {
    const res = await fetch('/admin/api/conversation/cancel', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        workspace_id: workspaceId,
        conversation_id: sessionId,
      }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to cancel agent: ${res.status} - ${text}`)
    }
    const data: CancelAgentResponse = await res.json()
    return data
  }

  static async getActiveRuns(workspaceId: string): Promise<ActiveRunsResponse> {
    const res = await fetch(API_ENDPOINTS.activeRuns(workspaceId))
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to load active runs: ${res.status} - ${text}`)
    }
    return res.json()
  }

  static async getGlobalActiveRuns(): Promise<GlobalActiveRunsResponse> {
    const res = await fetch(API_ENDPOINTS.activeRunsGlobal)
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to load running activity: ${res.status} - ${text}`)
    }
    return res.json()
  }

  // promoteQueuedRun serves a waiting external caller now, cancelling the run
  // that holds the model it needs. cancelQueuedRun drops it unserved. Both act
  // on a key from /admin/api/active-runs.
  static async promoteQueuedRun(key: string): Promise<void> {
    await this.queueAction(API_ENDPOINTS.queuePromote(key), 'promote', key)
  }

  static async cancelQueuedRun(key: string): Promise<void> {
    await this.queueAction(API_ENDPOINTS.queueCancel(key), 'cancel', key)
  }

  private static async queueAction(endpoint: string, action: string, key: string): Promise<void> {
    const res = await fetch(endpoint, { method: 'POST' })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to ${action} queued caller ${key}: ${res.status} - ${text}`)
    }
  }

  static async submitGuardrailDecision(decisionId: string, allow: boolean, persist: boolean): Promise<void> {
    const res = await fetch('/admin/api/conversation/guardrail-decision', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decision_id: decisionId, allow, persist }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`Failed to submit guardrail decision: ${res.status} - ${text}`)
    }
  }
}
