import type {
  ChatRequestPayload,
  ChatResponsePayload,
  CancelAgentResponse,
  SessionBrief,
  AssistantSession,
} from '../types/assistant'

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
}
