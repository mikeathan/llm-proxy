import { describe, it, expect } from 'vitest'
import { buildSegmentsFromHistory, groupTurns } from '../../../utils/message/turnGrouper'
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

describe('groupTurns with context-sieve messages', () => {
  it('keeps one run as one turn when the sieve note and progress ledger land in it', () => {
    const turns = groupTurns([
      { role: 'user', content: 'check the environment' },
      { role: 'assistant', content: 'Thought: start with node', tool_calls: [] },
      { role: 'user', content: '[System Note: History distilled to save context. Continue your task.]' },
      { role: 'user', content: '[Progress ledger — recorded by the system, not by you. Already done in this run:]\n✓ read_file a.txt' },
      { role: 'assistant', content: 'All checks passed.' },
    ])
    expect(turns).toHaveLength(1)
    expect(turns[0]!.userMessage).toBe('check the environment')
    expect(turns[0]!.finalAnswer).toBe('All checks passed.')
  })
})

describe('groupTurns with run records', () => {
  it('carries the run record stored on the user message onto its turn', () => {
    const run = { model: 'qwen3.6-35b', started_at: '2026-10-01T10:00:00Z', duration_ms: 42_000, prompt_tokens: 1200, completion_tokens: 85 }
    const turns = groupTurns([
      { role: 'user', content: 'old turn' },
      { role: 'assistant', content: 'old answer' },
      { role: 'user', content: 'new turn', run },
      { role: 'assistant', content: 'new answer' },
    ])
    expect(turns[0]!.run).toBeUndefined()
    expect(turns[1]!.run).toEqual(run)
  })
})
