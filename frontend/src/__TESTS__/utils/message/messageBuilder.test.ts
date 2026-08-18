import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { useMessageBuilder } from '../../../utils/message/messageBuilder'
import type { AssistantMessage, Segment } from '../../../types/assistant'
import type { AgentEvent } from '../../../types/dispatcher'

function stillThinkingEvent(elapsed = '5s'): AgentEvent {
  return {
    type: 'lifecycle',
    payload: { phase: 'still_thinking', elapsed } as unknown as AgentEvent['payload'],
  }
}

describe('useMessageBuilder still_thinking lifecycle', () => {
  it('flips idle -> thinking from a still_thinking event', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    expect(builder.phase.value).toBe('idle')
    builder.handleEvent(stillThinkingEvent())
    expect(builder.phase.value).toBe('thinking')
    expect(builder.thinking.value).toBe(true)
  })

  it('is a no-op after the turn is done', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.finalize('final answer')
    expect(builder.phase.value).toBe('done')
    expect(builder.thinking.value).toBe(false)

    builder.handleEvent(stillThinkingEvent())
    expect(builder.phase.value).toBe('done')
    expect(builder.thinking.value).toBe(false)
  })

  it('keeps a working turn in thinking (idempotent re-emit)', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(stillThinkingEvent())
    builder.handleEvent(stillThinkingEvent('10s'))
    expect(builder.phase.value).toBe('thinking')
    expect(builder.thinking.value).toBe(true)
  })
})

function upstreamEvent(overrides: Partial<{ attempt: number, max_attempts: number, reason: 'transport' | 'status', status: number, error: string, err_class: string }> = {}): AgentEvent {
  return {
    type: 'upstream',
    payload: {
      event: 'retry',
      reason: overrides.reason ?? 'transport',
      attempt: overrides.attempt ?? 2,
      max_attempts: overrides.max_attempts ?? 3,
      ...(overrides.error !== undefined ? { error: overrides.error } : {}),
      ...(overrides.err_class !== undefined ? { err_class: overrides.err_class } : {}),
      ...(overrides.status !== undefined ? { status: overrides.status } : {}),
    },
  }
}

function streamResumeEvent(): AgentEvent {
  return { type: 'reasoning', payload: 'thinking text' }
}

function lastSegments(messages: { value: AssistantMessage[] }): Segment[] {
  return messages.value[0]?.segments ?? []
}

describe('useMessageBuilder upstream notices', () => {
  it('appends a pending notice segment on an upstream retry event', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'status', status: 529, attempt: 2, max_attempts: 3 }))

    const segments = lastSegments(messages)
    const notice = segments[0]
    expect(segments.length).toBe(1)
    expect(notice?.kind).toBe('notice')
    if (notice?.kind === 'notice') {
      expect(notice.status).toBe('pending')
      expect(notice.message).toContain('2/3')
      expect(notice.message).toContain('529')
    }
  })

  it('does not disturb phase/thinking/streaming flags while a retry is pending', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent())
    expect(builder.phase.value).toBe('idle')
    expect(builder.thinking.value).toBe(false)
    expect(builder.streaming.value).toBe(false)
  })

  it('shows the classified err_class in the transport retry notice', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({
      err_class: 'connection-closed',
      error: 'Post "https://integrate.api.nvidia.com/v1/chat/completions": unexpected EOF',
    }))

    const notice = lastSegments(messages).find(s => s.kind === 'notice')
    expect(notice?.kind === 'notice' ? notice.message : '').toContain('connection-closed')
    expect(notice?.kind === 'notice' ? notice.message : '').toContain('unexpected EOF')
  })

  it('marks the pending notice resolved when streaming resumes', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent())
    builder.handleEvent(streamResumeEvent())

    const notice = lastSegments(messages).find(s => s.kind === 'notice')
    expect(notice?.kind === 'notice' ? notice.status : undefined).toBe('resolved')
  })

  it('keeps an error segment terminal even after pending notices', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent())
    builder.handleEvent({ type: 'error', payload: { error: 'llm completion failed' } as unknown as AgentEvent['payload'] })

    const segments = lastSegments(messages)
    const kinds = segments.map(s => s.kind)
    expect(kinds).toContain('notice')
    expect(kinds).toContain('error')
    expect(builder.phase.value).toBe('done')
  })

  it('resolves a pending notice when the run fails terminally', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent())
    builder.handleEvent({ type: 'error', payload: { error: 'retries exhausted' } as unknown as AgentEvent['payload'] })

    const notice = lastSegments(messages).find(s => s.kind === 'notice')
    expect(notice?.kind === 'notice' ? notice.status : undefined).toBe('resolved')
  })

  it('resolves all stacked pending notices, not just the latest', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ attempt: 1 }))
    builder.handleEvent({ type: 'reasoning', payload: 'interleaved thought' })
    builder.handleEvent(upstreamEvent({ attempt: 2 }))
    builder.handleEvent(streamResumeEvent())

    const notices = lastSegments(messages).filter(s => s.kind === 'notice')
    expect(notices.length).toBe(2)
    for (const n of notices) {
      expect(n.kind === 'notice' ? n.status : undefined).toBe('resolved')
    }
  })

  it('collapses consecutive pending retry notices into a single segment', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ attempt: 1 }))
    builder.handleEvent(upstreamEvent({ attempt: 2 }))
    builder.handleEvent(upstreamEvent({ attempt: 3 }))

    const notices = lastSegments(messages).filter(s => s.kind === 'notice')
    expect(notices.length).toBe(1)
    expect(notices[0]?.kind === 'notice' ? notices[0].message : undefined).toContain('3/3')
  })
})
