import type { AssistantMessage } from '../types/assistant'
import type { AgentTurnData, AgentStepData } from '../types/agent-run'

export function groupMessagesIntoTurns(messages: AssistantMessage[]): AgentTurnData[] {
  const turns: AgentTurnData[] = []
  let i = 0

  while (i < messages.length) {
    const first = messages[i]
    if (!first) { i++; continue }
    if (first.role === 'system') { i++; continue }
    if (first.role !== 'user') { i++; continue }

    const turn: AgentTurnData = {
      userMessage: first.content,
      userTimestamp: '',
      thinking: '',
      steps: [],
      finalAnswer: '',
      totalDurationMs: 0,
      state: 'completed',
      createdAt: new Date().toISOString(),
    }
    i++

    const thinkingParts: string[] = []
    let pendingStep: AgentStepData | null = null

    while (i < messages.length) {
      const msg = messages[i]
      if (!msg || msg.role === 'user') break
      i++

      if (msg.role === 'assistant') {
        if (msg.tool_calls && msg.tool_calls.length > 0) {
          for (const tc of msg.tool_calls) {
            if (pendingStep) {
              turn.steps.push(pendingStep)
              pendingStep = null
            }
            pendingStep = {
              toolName: tc.function.name || '',
              args: tc.function.arguments || '{}',
              result: '',
              status: 'success' as const,
              durationMs: 0,
            }
          }
          if (msg.content) thinkingParts.push(msg.content)
        } else if (msg.content) {
          let hasMoreAfter = false
          for (let j = i; j < messages.length; j++) {
            const next = messages[j]
            if (next?.role === 'user') break
            if (next?.role === 'assistant') { hasMoreAfter = true; break }
          }
          if (!hasMoreAfter) {
            turn.finalAnswer = msg.content
          } else {
            thinkingParts.push(msg.content)
          }
        }
      } else if (msg.role === 'tool' && pendingStep) {
        const tr = msg.toolResult || { result: msg.content, error: undefined }
        pendingStep.result = typeof tr.result === 'string' ? tr.result : JSON.stringify(tr.result, null, 2)
        if (tr.error) {
          pendingStep.error = tr.error
          pendingStep.status = 'error'
        } else {
          pendingStep.status = 'success'
        }
        turn.steps.push(pendingStep)
        pendingStep = null
      }
    }

    if (pendingStep) {
      turn.steps.push(pendingStep)
    }

    turn.thinking = stripToolCallXml(thinkingParts.join('\n\n'))
    turns.push(turn)
  }

  return turns
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 10000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.round(ms / 1000)}s`
}

/** Strip empty <tool_call> / <tool_result> XML artifacts from model thinking text. */
export function stripToolCallXml(text: string): string {
  return text.replace(/<tool_call>\s*<\/tool_call>/gi, '').replace(/<tool_result>\s*<\/tool_result>/gi, '').trim()
}

export function truncatePreview(text: string, max = 120): string {
  const firstLine = text.split('\n')[0] || ''
  if (firstLine.length <= max) return firstLine
  return firstLine.substring(0, max) + '…'
}
