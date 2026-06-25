import type { AssistantMessage, Segment } from '../types/assistant'

export interface Turn {
  userMessage: string
  agentOutput: string
  finalAnswer: string
  segments: Segment[]
  messages: AssistantMessage[]
}

export function groupTurns(messages: AssistantMessage[]): Turn[] {
  const turns: Turn[] = []
  let i = 0

  while (i < messages.length) {
    const m = messages[i]
    if (!m || m.role === 'system' || m.role !== 'user') { i++; continue }

    const turn: Turn = {
      userMessage: m.content,
      agentOutput: '',
      finalAnswer: '',
      segments: [],
      messages: [],
    }
    i++

    const assistantMsgs: AssistantMessage[] = []
    while (i < messages.length) {
      const msg = messages[i]
      if (!msg || msg.role === 'user') break
      if (msg.role === 'assistant' && !msg.tool_calls?.length) {
        assistantMsgs.push(msg)
        turn.messages.push(msg)
      }
      i++
    }

    if (assistantMsgs.length === 1) {
      const only = assistantMsgs[0]
      if (only) {
        turn.agentOutput = only.content
        if (only.segments) turn.segments = only.segments
      }
    } else if (assistantMsgs.length > 1) {
      const last = assistantMsgs[assistantMsgs.length - 1]
      if (last) turn.finalAnswer = last.content

      const first = assistantMsgs[0]
      if (first?.segments) turn.segments = first.segments

      turn.agentOutput = assistantMsgs.slice(0, -1)
        .map(m => m.content)
        .filter(Boolean)
        .join('\n')
    }

    turns.push(turn)
  }

  return turns
}
