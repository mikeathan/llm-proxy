export type AssistantRole = 'system' | 'user' | 'assistant' | 'tool'

export interface ToolCall {
  id: string
  type: string
  function: {
    name: string
    arguments: string
  }
}

export interface AssistantMessage {
  role: AssistantRole
  content: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  toolResult?: { name: string; result: any; error?: string }
}

export interface SessionBrief {
  id: string
  snippet: string
  updated_at: string
}

export interface AssistantSession {
  id: string
  workspace_id: string
  context_version: string
  timezone: string
  history: AssistantMessage[]
  updated_at: string
}

export interface ChatRequestPayload {
  workspace_id: string
  conversation_id?: string
  message: string
  context_version?: string
  timezone?: string
}

import type { AgentEvent } from './dispatcher'

export interface ChatResponsePayload {
  reply: string
  conversation_id: string
  events?: AgentEvent[]
}
