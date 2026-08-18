import type { AssistantMessage, Segment } from '../../types/assistant'
import type { Turn } from '../../types/message'

// Matches agent-internal control messages injected by the backend (format
// errors, retry prompts, incomplete-response warnings, automation nag/retry
// signals).  These use role:"user" so the model sees them as corrective
// feedback, but they must not create turn boundaries — or appear at all — in
// the UI.  New entries here must stay in sync with prompts/templates.go.
const nagRe = /^(You returned an incomplete|⚠️\s+WARNING:|The tool call format|Your tool call|FORMAT ERROR:|You exceeded|You must not|SYSTEM:|SYSTEM CRITICAL:|TOO LONG:|\[Retry after the previous model attempt failed or timed out\])/

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
  const unresolved: Extract<Segment, { kind: 'tool_call' }>[] = []

  // Drop control messages (retry/nag signals) so persisted automation history
  // never re-displays backend-internal directives in the chat pane.
  const result = messages.filter((m) => !isInternalMessage(m)).map((msg) => {
    if (msg.segments && msg.segments.length > 0) return msg

    const segments: Segment[] = []

    if (msg.reasoning_content) {
      segments.push({ kind: 'reasoning', text: msg.reasoning_content })
    }

    // A persisted terminal failure is stored as an assistant message with an
    // `error` field. Render it as an explicit error segment so a reloaded
    // failed run shows why it stopped instead of a blank turn.
    if (msg.error) {
      segments.push({ kind: 'error', message: msg.error })
    }

    // Content on assistant messages with tool calls is the model's planning
    // text, not output.  Move it to a reasoning segment so it doesn't show
    // as raw text.
    if (msg.role === 'assistant' && msg.tool_calls && msg.tool_calls.length > 0 && msg.content && !msg.reasoning_content) {
      segments.push({ kind: 'reasoning', text: msg.content })
    }

    if (msg.tool_calls) {
      for (const tc of msg.tool_calls) {
        const seg: Segment = { kind: 'tool_call', name: tc.function.name, args: tc.function.arguments, status: 'running' }
        segments.push(seg)
        idSegments.set(tc.id, seg)
        unresolved.push(seg)
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
        // Resolved — remove from unresolved list.
        const idx = unresolved.indexOf(seg)
        if (idx !== -1) unresolved.splice(idx, 1)
      }
    }

    return { ...msg, segments }
  })

  // History conversations have no actively-running tools.  Any tool-call
  // segment still marked 'running' was never matched to a result message
  // (missing tool_call_id, mismatched id, or result not persisted).  Default
  // to 'success' so the UI never shows a stuck spinner icon.
  for (const seg of unresolved) {
    if (seg.status === 'running') {
      seg.status = 'success'
    }
  }

  return result
}
