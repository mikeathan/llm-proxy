import type { AssistantMessage, Segment } from '../../types/assistant'

export interface Turn {
  userMessage: string
  finalAnswer: string
  segments: Segment[]
  messages: AssistantMessage[]
  canceled?: boolean
}

// Matches agent-internal nag messages injected by the backend (format errors,
// retry prompts, incomplete-response warnings).  These use role:"user" so the
// model sees them as corrective feedback, but they must not create turn
// boundaries in the UI.
const nagRe = /^(You returned an incomplete|⚠️\s+WARNING:|The tool call format|Your tool call|FORMAT ERROR:|You exceeded|You must not)/

function isInternalMessage(m: AssistantMessage): boolean {
  return m.role === 'user' && nagRe.test(m.content)
}

export function groupTurns(messages: AssistantMessage[]): Turn[] {
  const turns: Turn[] = []
  let i = 0

  while (i < messages.length) {
    const m = messages[i]
    if (!m || m.role === 'system' || isInternalMessage(m) || m.role !== 'user') { i++; continue }

    const turn: Turn = {
      userMessage: m.content,
      finalAnswer: '',
      segments: [],
      messages: [],
    }
    i++

    const assistantMsgs: AssistantMessage[] = []
    while (i < messages.length) {
      const msg = messages[i]
      if (!msg || (msg.role === 'user' && !isInternalMessage(msg))) break
      if (msg.role === 'assistant') {
        assistantMsgs.push(msg)
        turn.messages.push(msg)
      }
      i++
    }

    turn.canceled = m.canceled || assistantMsgs.some(msg => msg.canceled)

    if (assistantMsgs.length === 1) {
      const only = assistantMsgs[0]
      if (only) {
        if (only.segments) turn.segments = only.segments
        turn.finalAnswer = only.content
      }
    } else if (assistantMsgs.length > 1) {
      const last = assistantMsgs[assistantMsgs.length - 1]
      if (last) turn.finalAnswer = last.content

      // Aggregate segments from ALL assistant messages, not just the last.
      // The final summary message has the finalAnswer but often no segments
      // (no reasoning, no tool calls); intermediate messages carry them.
      turn.segments = assistantMsgs
        .flatMap(m => m.segments ?? [])
    }

    turns.push(turn)
  }

  return turns
}

// buildSegmentsFromHistory reconstructs the frontend-only `segments` field
// from persisted message fields (reasoning_content, tool_calls, tool results).
// The backend stores conversations but not the ephemeral segments built during streaming.
export function buildSegmentsFromHistory(messages: AssistantMessage[]): AssistantMessage[] {
  const idSegments = new Map<string, Segment>()

  return messages.map((msg) => {
    if (msg.segments && msg.segments.length > 0) return msg

    const segments: Segment[] = []

    if (msg.reasoning_content) {
      segments.push({ kind: 'reasoning', text: msg.reasoning_content })
    }

    // Content on assistant messages with tool calls is the model's planning
    // text, not output.  Move it to a reasoning segment so it doesn't show
    // as raw text.  Skip submit_final_answer — its content is the report.
    if (msg.role === 'assistant' && msg.tool_calls && msg.tool_calls.length > 0 && msg.content) {
      const isSubmit = msg.tool_calls.some(tc => tc.function.name === 'submit_final_answer')
      if (!isSubmit && !msg.reasoning_content) {
        segments.push({ kind: 'reasoning', text: msg.content })
      }
    }

    if (msg.tool_calls) {
      for (const tc of msg.tool_calls) {
        const seg: Segment = { kind: 'tool_call', name: tc.function.name, args: tc.function.arguments, status: 'running' }
        segments.push(seg)
        idSegments.set(tc.id, seg)
      }
    }

    // Tool result messages (role: 'tool') carry status and result in the content field.
    // The backend stores the marshalled result as-is; errors produce {"error": "..."}.
    if (msg.role === 'tool' && msg.tool_call_id && msg.content) {
      const seg = idSegments.get(msg.tool_call_id)
      if (seg && seg.kind === 'tool_call') {
        let parsed: any
        try { parsed = JSON.parse(msg.content) } catch { parsed = msg.content }
        if (parsed && typeof parsed === 'object' && parsed.error) {
          seg.status = 'error'
          seg.error = String(parsed.error)
        } else {
          seg.status = 'success'
          seg.result = typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2)
        }
      }
    }

    return { ...msg, segments }
  })
}
