import { describe, it, expect } from 'vitest'
import { buildSegmentsFromHistory } from '../../../utils/message/turnGrouper'
import type { AssistantMessage } from '../../../types/assistant'

describe('buildSegmentsFromHistory', () => {
  it('maps a persisted terminal error to an error segment', () => {
    const history: AssistantMessage[] = [
      { role: 'user', content: 'list all files' },
      {
        role: 'assistant',
        content: '',
        error: 'llm completion failed: Post "https://upstream": unexpected EOF',
      },
    ]

    const [userMsg, errMsg] = buildSegmentsFromHistory(history)

    expect(userMsg!.segments).toEqual([])
    expect(errMsg!.error).toBe('llm completion failed: Post "https://upstream": unexpected EOF')
    expect(errMsg!.segments).toEqual([
      { kind: 'error', message: 'llm completion failed: Post "https://upstream": unexpected EOF' },
    ])
  })

  it('keeps an assistant message with no error free of an error segment', () => {
    const history: AssistantMessage[] = [
      { role: 'user', content: 'hi' },
      { role: 'assistant', content: 'a normal reply' },
    ]

    const [, reply] = buildSegmentsFromHistory(history)
    expect(reply!.segments).toEqual([])
  })

  it('preserves an error segment alongside existing reasoning segments', () => {
    const history: AssistantMessage[] = [
      {
        role: 'assistant',
        content: '',
        reasoning_content: 'checking the file',
        error: 'upstream timed out',
      },
    ]

    const [msg] = buildSegmentsFromHistory(history)
    expect(msg!.segments).toEqual([
      { kind: 'reasoning', text: 'checking the file' },
      { kind: 'error', message: 'upstream timed out' },
    ])
  })

  it('does not render internal control messages, even with content', () => {
    const history: AssistantMessage[] = [
      { role: 'user', content: 'SYSTEM: retry directive' },
      { role: 'user', content: 'real prompt' },
    ]

    const result = buildSegmentsFromHistory(history)
    expect(result).toHaveLength(1)
    expect(result[0]!.content).toBe('real prompt')
  })
})
