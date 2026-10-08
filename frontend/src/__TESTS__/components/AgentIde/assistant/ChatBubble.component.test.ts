import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ChatBubble from '../../../../components/AgentIde/assistant/ChatBubble.vue'
import type { Turn } from '../../../../types/message'

const MarkdownViewerStub = {
  props: ['content', 'variant'],
  template: '<div class="md-stub" :data-variant="variant">{{ content }}</div>',
}

const stubs = {
  MarkdownViewer: MarkdownViewerStub,
  Icon: true,
  ArcOrbitLoader: true,
  ToolCallSegment: true,
}

function turn(overrides: Partial<Turn> = {}): Turn {
  return {
    userMessage: 'list files',
    finalAnswer: '',
    segments: [],
    messages: [],
    ...overrides,
  }
}

function mountBubble(props: Partial<Record<string, unknown>> = {}) {
  return mount(ChatBubble, {
    props: {
      turn: turn(),
      idx: 0,
      loading: true,
      thinking: true,
      liveReasoning: '',
      paused: false,
      isLastTurn: true,
      phase: 'thinking',
      isInsetCollapsed: false,
      isSegExpanded: () => false,
      ...props,
    },
    global: { stubs },
  })
}

describe('ChatBubble live reasoning gating', () => {
  it('renders live reasoning (via MarkdownViewer) in the last (live) turn while streaming', async () => {
    const wrapper = mountBubble({ liveReasoning: 'new run thinking…', isLastTurn: true, phase: 'thinking' })
    expect(wrapper.find('.inset-reasoning--live').exists()).toBe(true)
    // Live reasoning renders with full markdown again (GPU audit confirmed
    // markdown is not a GPU driver; the plain-text experiment was reverted).
    expect(wrapper.find('.inset-reasoning--live').text()).toContain('new run thinking…')
  })

  it('does NOT render live reasoning text in a historical (non-last) turn, even while a new run streams', async () => {
    // Regression: after retry, the new run's shared liveReasoning ref used to
    // bleed into the previous (errored/cancelled) turn's expanded inset.
    const wrapper = mountBubble({
      liveReasoning: 'new run thinking…',
      isLastTurn: false,
      idx: 0,
      phase: 'thinking',
    })
    // The inset (and the live block inside it) is kept mounted but hidden via
    // v-show so collapse/expand mid-stream is flicker-free — assert hidden
    // (display:none), not absent.
    const live = wrapper.find('.inset-reasoning--live')
    expect(live.exists()).toBe(true)
    expect(live.attributes('style')).toContain('display: none')
  })

  it('still renders committed error segments in a historical turn', async () => {
    const wrapper = mountBubble({
      turn: turn({ segments: [{ kind: 'error', message: 'connection refused' }] }),
      liveReasoning: 'new run thinking…',
      isLastTurn: false,
      idx: 0,
      phase: 'thinking',
    })
    // The committed error segment stays visible; only the live text is gated.
    expect(wrapper.find('.inset-error').exists()).toBe(true)
    // The live-reasoning block is v-show'd (kept in the DOM to avoid
    // expand/collapse flicker) — it must be hidden via display:none, not shown.
    const live = wrapper.find('.inset-reasoning--live')
    expect(live.exists()).toBe(true)
    expect(live.attributes('style')).toContain('display: none')
    // ...and the new run's live text must not be visible in it.
    expect(wrapper.find('.inset-reasoning--live').text()).toContain('new run thinking…')
  })
})

describe('ChatBubble copy', () => {
  it('offers copying a finished answer as its markdown', () => {
    const wrapper = mountBubble({ turn: turn({ finalAnswer: '## Done\n- a' }), loading: false, thinking: false, phase: 'done' })
    const copy = wrapper.findComponent({ name: 'CopyButton' })
    expect(copy.exists()).toBe(true)
    expect(copy.props('text')).toBe('## Done\n- a')
    expect(copy.props('title')).toBe('Copy answer')
  })

  it('offers nothing to copy while the answer is still streaming', () => {
    const wrapper = mountBubble({ turn: turn({ finalAnswer: 'partial' }), loading: true, isLastTurn: true, phase: 'generating' })
    expect(wrapper.findComponent({ name: 'CopyButton' }).exists()).toBe(false)
  })
})

const tool = (name: string) => ({ kind: 'tool_call' as const, name, args: '{}', status: 'success' as const })

describe('ChatBubble layout', () => {
  afterEach(() => vi.useRealTimers())

  const finished = (over: Partial<Record<string, unknown>> = {}) =>
    mountBubble({ loading: false, thinking: false, isLastTurn: false, phase: 'done', isInsetCollapsed: true, ...over })

  it('sums a finished turn up in one line that opens its steps', () => {
    const w = finished({
      turn: turn({ finalAnswer: 'Answer', segments: [{ kind: 'reasoning', text: 'plan' }, tool('read_file'), tool('internet_search')] }),
    })
    const line = w.get('button.activity-line')
    expect(line.text()).toContain('2 steps')
    expect(line.attributes('aria-expanded')).toBe('false')
    expect(w.findAll('ol.timeline > :not(.inset-reasoning--live)')).toHaveLength(3)
    expect(w.get('ol.timeline').element.closest('.bubble-inset-wrap')?.classList.contains('collapsed')).toBe(true)
  })

  it('has no activity line when a turn answered without any work', () => {
    expect(finished({ turn: turn({ finalAnswer: 'Hi there' }) }).find('button.activity-line').exists()).toBe(false)
  })

  it('renders the answer as a document and reasoning compact', () => {
    const w = finished({ isInsetCollapsed: false, turn: turn({ finalAnswer: 'Answer', segments: [{ kind: 'reasoning', text: 'plan' }] }) })
    expect(w.get('.turn-answer .md-stub').attributes('data-variant')).toBe('document')
    expect(w.get('.inset-reasoning .md-stub').attributes('data-variant')).toBe('compact')
  })

  it('leaves no empty thinking row under a finished turn', () => {
    expect(finished({ turn: turn({ finalAnswer: 'Answer' }) }).find('.bubble-paused').exists()).toBe(false)
  })

  it('keeps the thinking row reserved while the live turn runs', () => {
    const w = mountBubble({ turn: turn({ segments: [tool('read_file')] }), loading: true, isLastTurn: true, phase: 'working', paused: false })
    expect(w.find('.bubble-paused').classes()).toContain('bubble-paused--hidden')
  })

  it('says how long a turn watched live worked, once it finishes', async () => {
    vi.useFakeTimers()
    const w = mountBubble({ turn: turn({ segments: [tool('read_file')] }), loading: true, isLastTurn: true, phase: 'working' })
    expect(w.get('button.activity-line').text()).toContain('Working · 1 step')
    await vi.advanceTimersByTimeAsync(75_000)
    expect(w.get('.activity-time').text()).toBe('1m 15s')
    await w.setProps({ loading: false, phase: 'done', turn: turn({ finalAnswer: 'Done', segments: [tool('read_file')] }) })
    expect(w.get('button.activity-line').text()).toContain('Worked 1m 15s · 1 step')
  })

  it('shows an older turn as finished while a new run is live', () => {
    const w = mountBubble({ turn: turn({ finalAnswer: 'Earlier answer' }), loading: true, isLastTurn: false, phase: 'thinking' })
    expect(w.get('.turn-answer').text()).toContain('Earlier answer')
  })

  it('drops a reasoning step\'s own "Thought:" prefix, the step already says it', () => {
    const w = finished({ isInsetCollapsed: false, turn: turn({ finalAnswer: 'A', segments: [{ kind: 'reasoning', text: 'Thought: check node first' }] }) })
    expect(w.get('.inset-reasoning .md-stub').text()).toBe('check node first')
  })
})

describe('ChatBubble run record', () => {
  const run = { model: 'qwen3.6-35b', started_at: '2026-10-01T10:00:00Z', duration_ms: 42_400, prompt_tokens: 1200, completion_tokens: 85 }
  const reloaded = (over: Partial<Record<string, unknown>> = {}) =>
    mountBubble({ loading: false, thinking: false, isLastTurn: true, phase: 'done', isInsetCollapsed: true, ...over })

  it('says how long a reloaded turn worked, from its stored record', () => {
    const w = reloaded({ turn: turn({ finalAnswer: 'A', segments: [tool('read_file'), tool('internet_search')], run }) })
    expect(w.get('button.activity-line').text()).toContain('Worked 42s · 2 steps')
  })

  it('names the model and the tokens it generated under the answer', () => {
    const meta = reloaded({ turn: turn({ finalAnswer: 'A', run }) }).get('.turn-meta')
    expect(meta.text()).toContain('qwen3.6-35b')
    expect(meta.text()).toContain('85 tokens')
    expect(meta.get('[title]').attributes('title')).toBe('85 tokens generated · 1.2K prompt tokens processed')
  })

  it('shows only what was recorded', () => {
    expect(reloaded({ turn: turn({ finalAnswer: 'A' }) }).find('.turn-meta').exists()).toBe(false)
    const noTokens = reloaded({ turn: turn({ finalAnswer: 'A', run: { model: 'gpt-x', duration_ms: 1000 } }) }).get('.turn-meta')
    expect(noTokens.text()).toBe('gpt-x')
  })

  // An explicit "remember …" is saved by the backend before the run starts; the turn says so, and it survives a reload.
  it('says what was saved to memory from the message, and nothing when nothing was', () => {
    const saved = reloaded({ turn: turn({ finalAnswer: 'A', run: { memory_saved: ['the staging DB runs on port 5433', 'answer briefly'] } }) }).get('[data-test="memory-saved"]')
    expect(saved.text()).toContain('Saved to memory')
    expect(saved.findAll('li').map((li) => li.text())).toEqual(['the staging DB runs on port 5433', 'answer briefly'])
    expect(reloaded({ turn: turn({ finalAnswer: 'A', run: { model: 'gpt-x' } }) }).find('[data-test="memory-saved"]').exists()).toBe(false)
    expect(reloaded({ turn: turn({ finalAnswer: 'A' }) }).find('[data-test="memory-saved"]').exists()).toBe(false)
  })
})
