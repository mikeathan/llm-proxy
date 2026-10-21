import { describe, it, expect, vi } from 'vitest'
import { ref } from 'vue'
import { MODEL_BUSY_NOTICE, MODEL_STARTING_NOTICE } from '../../../constants/labels'
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

function upstreamEvent(overrides: Partial<{ attempt: number, max_attempts: number, reason: 'transport' | 'status' | 'model_starting' | 'model_busy', status: number, error: string, err_class: string }> = {}): AgentEvent {
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

  it('opens a Wait/Cancel prompt on a residency refusal', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'model_busy', error: 'the local model is serving a for a running job' }))

    expect(builder.modelBusy.value).toBe('the local model is serving a for a running job')
    const notice = lastSegments(messages).find(s => s.kind === 'notice')
    expect(notice?.kind === 'notice' ? notice.message : '').toContain('the local model is serving')
  })

  it('leaves the model-busy prompt unset for ordinary retries', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'status', status: 529 }))

    expect(builder.modelBusy.value).toBeNull()
  })

  it('clears the model-busy prompt when the stream resumes', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'model_busy', error: 'busy' }))
    builder.handleEvent(streamResumeEvent())

    expect(builder.modelBusy.value).toBeNull()
  })

  it('clears the model-busy prompt on dismiss', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'model_busy', error: 'busy' }))
    builder.dismissModelBusy()

    expect(builder.modelBusy.value).toBeNull()
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

  it('explains a model-starting upstream instead of a retry attempt chip', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'model_starting', status: 503, attempt: 1, max_attempts: 0 }))

    const notice = lastSegments(messages)[0]
    expect(notice?.kind === 'notice' ? notice.message : undefined).toBe(MODEL_STARTING_NOTICE)
    if (notice?.kind === 'notice') {
      expect(notice.message).not.toContain('1/0')
      expect(notice.message).not.toContain('transport')
    }
  })

  it('explains a busy model with the server detail instead of a status code', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    const detail = 'the local model is serving a for a running job; b would interrupt it'
    builder.handleEvent(upstreamEvent({ reason: 'model_busy', status: 429, error: detail }))

    const notice = lastSegments(messages)[0]
    expect(notice?.kind === 'notice' ? notice.message : undefined).toContain(detail)
    if (notice?.kind === 'notice') {
      // The server detail stands alone — no second "model in use" prefix.
      expect(notice.message).toBe(detail)
      expect(notice.message).not.toContain('429')
      expect(notice.message).not.toContain('Upstream retrying')
    }
  })

  it('states a busy model without a detail rather than showing nothing', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(upstreamEvent({ reason: 'model_busy', status: 429 }))

    const notice = lastSegments(messages)[0]
    expect(notice?.kind === 'notice' ? notice.message : undefined).toBe(MODEL_BUSY_NOTICE)
  })
})

describe('useMessageBuilder tool-call-markup message guard', () => {
  function markupMessage(content: string): AgentEvent {
    return {
      type: 'message',
      payload: { role: 'assistant', content } as unknown as AgentEvent['payload'],
    }
  }

  it('does NOT finalize on a message whose content is a malformed <tool_call> attempt', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(markupMessage('<tool_call>\n{"list_directory", {"path": "."}}\n</tool_call>'))

    // No answer message was pushed and the turn is not done — the raw JSON
    // must not become the result (regression: content-only message finalize
    // heuristic treated the failed attempt as the final answer).
    expect(messages.value.some((m) => m.role === 'assistant' && m.content)).toBe(false)
    expect(builder.phase.value).not.toBe('done')
  })

  it('still finalizes on a genuine content-only message', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)

    builder.handleEvent(markupMessage('The full report is ready.'))

    expect(messages.value.some((m) => m.role === 'assistant' && m.content === 'The full report is ready.')).toBe(true)
    expect(builder.phase.value).toBe('done')
  })
})

describe('useMessageBuilder lifecycle completed markup guard', () => {
  function completedLifecycle(content: string): AgentEvent {
    return {
      type: 'lifecycle',
      payload: { phase: 'completed', content } as unknown as AgentEvent['payload'],
    }
  }

  it('does not finalize with tool-call markup from a completed lifecycle', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages, { finalizeOn: 'lifecycle' })

    builder.handleEvent(completedLifecycle('<tool_call>\n{"tool": "list_directory",\n  "args": {\n    "path": "."\n  }\n</tool_call>'))

    const answers = messages.value.filter((m) => m.role === 'assistant' && m.content)
    expect(answers.every((a) => !/<tool_call/i.test(a.content))).toBe(true)
    expect(builder.phase.value).toBe('done')
  })

  it('still finalizes with a genuine completed-lifecycle report', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages, { finalizeOn: 'lifecycle' })

    builder.handleEvent(completedLifecycle('The report is ready.'))

    expect(messages.value.some((m) => m.role === 'assistant' && m.content === 'The report is ready.')).toBe(true)
    expect(builder.phase.value).toBe('done')
  })
})

// A stream the backend cuts for looping in its thinking is restarted. The text already streamed must stay on screen
// and the restart must say why it paused, instead of the turn emptying with no explanation.
describe('useMessageBuilder stuck_detected lifecycle', () => {
  const stuck = (): AgentEvent => ({ type: 'lifecycle', payload: { phase: 'stuck_detected', reasoning_chars: 9000 } as unknown as AgentEvent['payload'] })
  const reasoning = (text: string): AgentEvent => ({ type: 'reasoning', payload: text as unknown as AgentEvent['payload'] })
  const segmentsOf = (messages: { value: AssistantMessage[] }) => (messages.value[0]?.segments ?? []) as Segment[]

  it('keeps the streamed reasoning and shows a pending notice', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)
    builder.handleEvent(reasoning('first I will check the page'))
    builder.handleEvent(stuck())

    const segments = segmentsOf(messages)
    expect(segments.some((s) => s.kind === 'reasoning' && s.text.includes('check the page'))).toBe(true)
    const notice = segments.find((s) => s.kind === 'notice')
    expect(notice?.kind === 'notice' && notice.status).toBe('pending')
    expect(builder.liveReasoning.value).toBe('')
  })

  it('resolves the notice when the restarted stream resumes, and keeps the old reasoning before the new', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)
    builder.handleEvent(reasoning('first attempt'))
    builder.handleEvent(stuck())
    vi.useFakeTimers()
    builder.handleEvent(reasoning('second attempt'))
    vi.advanceTimersByTime(1000) // the live text is flushed on a timer
    vi.useRealTimers()

    const segments = segmentsOf(messages)
    const notice = segments.find((s) => s.kind === 'notice')
    expect(notice?.kind === 'notice' && notice.status).toBe('resolved')
    expect(builder.liveReasoning.value).toContain('second attempt')
    const order = segments.map((s) => s.kind)
    expect(order.indexOf('reasoning')).toBeLessThan(order.indexOf('notice'))
  })

  it('resolves the notice when the restart goes straight to a tool call, or the turn ends', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)
    builder.handleEvent(reasoning('thinking'))
    builder.handleEvent(stuck())
    builder.handleEvent({ type: 'tool_call', payload: { id: 'c1', type: 'function', function: { name: 'read_file', arguments: '{}' } } as unknown as AgentEvent['payload'] })
    const notice = segmentsOf(messages).find((s) => s.kind === 'notice')
    expect(notice?.kind === 'notice' && notice.status).toBe('resolved')

    builder.handleEvent(stuck())
    builder.finalize('answer')
    const pending = segmentsOf(messages).filter((s) => s.kind === 'notice' && s.status === 'pending')
    expect(pending).toHaveLength(0)
  })

  it('does nothing visible on a turn that already finished', () => {
    const messages = ref<AssistantMessage[]>([])
    const builder = useMessageBuilder(messages)
    builder.finalize('done')
    builder.handleEvent(stuck())
    expect(segmentsOf(messages).some((s) => s.kind === 'notice')).toBe(false)
  })
})
